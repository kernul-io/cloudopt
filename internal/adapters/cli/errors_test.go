package cli_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kernul-io/cloudopt/internal/adapters/cli"
	"github.com/kernul-io/cloudopt/internal/adapters/config"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/domain/exitcodes"
)

func TestExitCode_mapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, exitcodes.Success},
		{"validation", &config.ValidationError{Fields: []string{"x"}}, exitcodes.InvalidInput},
		{"not_implemented", api.ErrNotImplemented("collect"), exitcodes.Success},
		{"canceled", context.Canceled, exitcodes.GeneralError},
		{"general", errors.New("boom"), exitcodes.GeneralError},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cli.ExitCode(tc.err); got != tc.want {
				t.Errorf("ExitCode() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCollectionAndAnalysisExitCodes(t *testing.T) {
	if got := cli.CollectionExitCode(api.ErrNotImplemented("collect")); got != exitcodes.Success {
		t.Errorf("collect not implemented exit = %d", got)
	}
	if got := cli.CollectionExitCode(errors.New("fail")); got != exitcodes.CollectionFail {
		t.Errorf("collect fail exit = %d", got)
	}
	if got := cli.AnalysisExitCode(errors.New("fail")); got != exitcodes.AnalysisFail {
		t.Errorf("analyze fail exit = %d", got)
	}
}
