package cli_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/rs/zerolog"

	"github.com/kernul-io/cloudopt/internal/adapters/cli"
	"github.com/kernul-io/cloudopt/internal/adapters/config"
	"github.com/kernul-io/cloudopt/internal/application/api"
)

func TestRunnerExecute_respectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := &cli.Runner{
		Logger: zerolog.New(io.Discard),
		Run:    api.NewRuntime(config.Settings{WorkspaceDir: t.TempDir()}),
	}

	err := runner.Execute(ctx, func(ctx context.Context) error {
		return runner.Run.Init(ctx)
	})
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestInit_doesNotOverwriteExistingConfig(t *testing.T) {
	dir := t.TempDir()
	settings := config.Settings{
		WorkspaceDir: dir,
		ConfigDir:    dir,
		DataDir:      dir,
		ReportsDir:   dir,
		TempDir:      dir,
		LogFormat:    "text",
		LogLevel:     "info",
	}
	rt := api.NewRuntime(settings)

	if err := rt.Init(context.Background()); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if err := rt.Init(context.Background()); err == nil {
		t.Fatal("second init should fail")
	}
}

func TestExecute_versionJSON(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"cloudopt", "version"}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := cli.Execute(&cli.Config{})
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)

	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if len(out) == 0 || out[0] != '{' {
		t.Fatalf("expected JSON on stdout, got %q", out)
	}
}

func TestExecute_collectNotImplemented(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	dir := t.TempDir()
	os.Args = []string{"cloudopt", "collect", "--workspace-dir", dir}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	code := cli.Execute(&cli.Config{})
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)

	if code != 0 {
		t.Fatalf("exit code %d, want 0 for not_implemented", code)
	}
	if !bytes.Contains(out, []byte(`"status":"not_implemented"`)) {
		t.Fatalf("stdout = %s", out)
	}
}
