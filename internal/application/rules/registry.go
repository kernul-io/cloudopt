package rules

import (
	"fmt"
	"sync"

	"github.com/kernul-io/cloudopt/internal/application/pricing"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// Evaluator performs a deterministic check against a snapshot view.
type Evaluator interface {
	Name() string
	Evaluate(ctx *SnapshotView, rule RuleSpec) EvaluatorResult
}

// EvaluatorResult is the outcome of one rule evaluation.
type EvaluatorResult struct {
	Findings       []CandidateFinding
	NotEvaluated   bool
	Reason         string
	ConfidenceNote string
}

// CandidateFinding is pre-suppression finding data from an evaluator.
type CandidateFinding struct {
	Title       string
	Description string
	ResourceIDs []types.ResourceID
	Evidence    []EvidenceDraft
	Assumptions []string
	Confidence  types.Percentage
	Savings     *SavingsDraft
}

// SavingsDraft becomes recommendation savings fields when persisted.
type SavingsDraft struct {
	Estimate          domain.SavingsEstimate
	InvestigationOnly bool
}

// EvidenceDraft becomes domain.Evidence when persisted on an analysis run.
type EvidenceDraft struct {
	Kind       domain.EvidenceKind
	ResourceID types.ResourceID
	Summary    string
	Detail     map[string]string
}

// Registry maps evaluator names to implementations.
type Registry struct {
	mu     sync.RWMutex
	byName map[string]Evaluator
}

func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Evaluator)}
}

func (r *Registry) Register(ev Evaluator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byName[ev.Name()] = ev
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.byName[name]
	return ok
}

func (r *Registry) Get(name string) (Evaluator, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ev, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("evaluator %q not registered", name)
	}
	return ev, nil
}

// DefaultRegistry returns a registry with built-in evaluators.
func DefaultRegistry(catalog *pricing.Catalog) *Registry {
	reg := NewRegistry()
	reg.Register(&StoppedInstanceStorageCost{})
	reg.Register(&UnattachedBlockVolume{})
	reg.Register(&StaleVolumeSnapshot{})
	reg.Register(&MissingCostAllocationTags{})
	reg.Register(&EC2DownsizeCandidate{Catalog: catalog})
	reg.Register(&EC2IdleInstance{Catalog: catalog})
	reg.Register(&EBSVolumeTypeOptimize{Catalog: catalog})
	reg.Register(&RDSDownsizeCandidate{Catalog: catalog})
	reg.Register(&NATGatewayLowUtilization{Catalog: catalog})
	return reg
}
