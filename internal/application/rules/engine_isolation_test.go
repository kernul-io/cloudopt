package rules_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/rules"
)

type panicEvaluator struct{}

func (panicEvaluator) Name() string { return "panic_evaluator" }

func (panicEvaluator) Evaluate(_ *rules.SnapshotView, _ rules.RuleSpec) rules.EvaluatorResult {
	panic("boom")
}

func TestPartialRuleFailureIsolated(t *testing.T) {
	reg := rules.NewRegistry()
	reg.Register(panicEvaluator{})
	reg.Register(&rules.StoppedInstanceStorageCost{})

	manifest := &rules.Manifest{
		RulesetVersion: "test",
		Rules: []rules.RuleSpec{
			{
				ID: "good", Version: "1", Title: "t", Category: "c", Severity: "low",
				Evaluator: "stopped_instance_storage_cost", Enabled: true,
				RequiredSignals: []string{"costs"},
			},
			{
				ID: "bad", Version: "1", Title: "t", Category: "c", Severity: "low",
				Evaluator: "panic_evaluator", Enabled: true,
				RequiredSignals: []string{"resources"},
			},
		},
	}

	snap := &domain.CollectionSnapshot{
		ID:          "snap",
		Status:      domain.SnapshotComplete,
		StartedAt:   mustTS(t, "2026-01-15T12:00:00Z"),
		CompletedAt: ptrTS(mustTS(t, "2026-01-15T12:00:00Z")),
		Resources: []domain.Resource{
			{ID: "res-i-stopped", Kind: domain.KindComputeInstance, State: "stopped"},
		},
		Costs: []domain.CostRecord{
			{ResourceID: "res-i-stopped", Amount: types.FromMajorUnits(5, "USD", 100)},
		},
	}

	out, err := rules.Engine{}.Analyze(rules.AnalyzeInput{Snapshot: snap, Manifest: manifest, Registry: reg})
	require.NoError(t, err)
	require.Equal(t, 1, out.Summary.NotEvaluated)
	require.Equal(t, 1, out.Summary.Failed)
}
