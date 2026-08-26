package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventKind classifies auditable workspace actions.
type EventKind string

const (
	EventCollectionScope EventKind = "collection_scope"
	EventAnalysisRun     EventKind = "analysis_run"
	EventSuppression     EventKind = "suppression"
	EventExport          EventKind = "export"
	EventAIApproval      EventKind = "ai_approval"
	EventDeletion        EventKind = "deletion"
	EventBackup          EventKind = "backup"
	EventRestore         EventKind = "restore"
)

// Event is one append-only audit record (no customer resource payloads).
type Event struct {
	Time        time.Time         `json:"time"`
	Kind        EventKind         `json:"kind"`
	Actor       string            `json:"actor,omitempty"`
	Workspace   string            `json:"workspace,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
	AnalyzerVer string            `json:"analyzer_version,omitempty"`
}

// Log appends JSONL audit records under the workspace.
type Log struct {
	path string
	mu   sync.Mutex
}

// NewLog opens or creates an audit log at workspace/data/audit.jsonl unless path override is set.
func NewLog(workspaceDir, overridePath string) (*Log, error) {
	path := overridePath
	if path == "" {
		path = filepath.Join(workspaceDir, "data", "audit.jsonl")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("audit log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	_ = f.Close()
	return &Log{path: path}, nil
}

// Append writes one event atomically.
func (l *Log) Append(ev Event) error {
	if l == nil {
		return nil
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now().UTC()
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// Path returns the on-disk audit log location.
func (l *Log) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}
