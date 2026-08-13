package ports

import "context"

// CommandRunner executes operational CLI workflows (collect, analyze, report).
type CommandRunner interface {
	Init(ctx context.Context) error
	Collect(ctx context.Context) error
	Analyze(ctx context.Context) error
	Report(ctx context.Context) error
}
