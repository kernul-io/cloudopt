package cli_test

import (
	"strings"
	"testing"

	"github.com/kernul-io/cloudopt/internal/adapters/cli"
)

func TestFormatResult_notImplementedJSON(t *testing.T) {
	out, err := cli.FormatResult(cli.Result{
		Status:  cli.StatusNotImplemented,
		Command: "collect",
		Message: `"collect" is not implemented yet`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"status":"not_implemented"`) {
		t.Fatalf("unexpected JSON: %s", out)
	}
}
