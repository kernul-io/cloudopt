package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"

	"github.com/kernul-io/cloudopt/internal/adapters/config"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// Runner executes commands with signal-aware cancellation.
type Runner struct {
	Logger zerolog.Logger
	Run    ports.CommandRunner
}

// Execute runs fn under a signal-notified context.
func (r *Runner) Execute(parent context.Context, fn func(context.Context) error) error {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return fn(ctx)
}

// LoadSettings loads configuration using shared CLI overrides.
func LoadSettings(over config.Overrides) (config.Settings, error) {
	settings, err := config.Load(over)
	if err != nil {
		return config.Settings{}, fmt.Errorf("load configuration: %w", err)
	}
	return settings, nil
}
