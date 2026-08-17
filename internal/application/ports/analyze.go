package ports

import "github.com/kernul-io/cloudopt/internal/domain/types"

// AnalyzeOptions configures a single analyze invocation from the CLI.
type AnalyzeOptions struct {
	SnapshotID           types.SnapshotID
	RuleIDs              []string
	Categories           []string
	Persist              bool
	JSONDetail           bool
	AllowPartialSnapshot bool
}
