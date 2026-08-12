package ports

import "context"

// CommandRunner executes operational CLI workflows (collect, analyze, report).
type CommandRunner interface {
	Init(ctx context.Context) error
	Collect(ctx context.Context, opts CollectOptions) (*CollectResult, error)
	Analyze(ctx context.Context, opts AnalyzeOptions) error
	Report(ctx context.Context, opts ReportOptions) error
}
