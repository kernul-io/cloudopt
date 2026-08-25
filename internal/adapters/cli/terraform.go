package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/terraform"
	"github.com/kernul-io/cloudopt/internal/domain/exitcodes"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func newTerraformCommand(cfg *Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "terraform",
		Short: "Correlate cloud inventory with Terraform state/plan JSON (read-only, no terraform CLI)",
	}
	cmd.AddCommand(newTerraformCorrelateCommand(cfg))
	return cmd
}

func newTerraformCorrelateCommand(cfg *Config) *cobra.Command {
	var stateJSON, planJSON, mappings, snapshotID, analysisRunID, jsonOut, markdownOut string
	var enrichFindings, strictPlan, jsonStdout bool
	cmd := &cobra.Command{
		Use:   "correlate",
		Short: "Match inventory resources to Terraform addresses and optional plan forecast",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTerraformCorrelate(cmd, cfg, api.TerraformCorrelateOptions{
				SnapshotID:     types.SnapshotID(snapshotID),
				AnalysisRunID:  types.AnalysisRunID(analysisRunID),
				StateJSONPath:  stateJSON,
				PlanJSONPath:   planJSON,
				MappingsPath:   mappings,
				JSONOut:        jsonOut,
				MarkdownOut:    markdownOut,
				StrictPlan:     strictPlan,
				EnrichFindings: enrichFindings,
			}, jsonStdout)
		},
	}
	cmd.Flags().StringVar(&stateJSON, "state-json", "", "Path to terraform show -json state output (required)")
	cmd.Flags().StringVar(&planJSON, "plan-json", "", "Path to terraform show -json plan output (optional forecast)")
	cmd.Flags().StringVar(&mappings, "mappings", "", "YAML file with explicit resource_id to terraform_address mappings")
	cmd.Flags().StringVar(&snapshotID, "snapshot-id", "", "Snapshot for inventory correlation (default: latest complete when enriching)")
	cmd.Flags().StringVar(&analysisRunID, "analysis-run-id", "", "Analysis run for finding enrichment")
	cmd.Flags().StringVar(&jsonOut, "json-out", "", "Write full correlation JSON to path")
	cmd.Flags().StringVar(&markdownOut, "markdown-out", "", "Write PR-comment Markdown summary to path")
	cmd.Flags().BoolVar(&enrichFindings, "enrich-findings", false, "Attach IaC context to analysis findings (requires snapshot)")
	cmd.Flags().BoolVar(&strictPlan, "strict-plan", false, "Exit non-zero when plan policy warnings are present")
	cmd.Flags().BoolVar(&jsonStdout, "json", false, "Emit correlation JSON on stdout")
	_ = cmd.MarkFlagRequired("state-json")
	return cmd
}

func runTerraformCorrelate(cmd *cobra.Command, cfg *Config, opts api.TerraformCorrelateOptions, jsonStdout bool) error {
	settings, err := LoadSettings(cfg.Overrides)
	if err != nil {
		_ = EmitError("terraform correlate", err.Error())
		return &ExitError{Code: exitcodes.InvalidInput, Err: err}
	}
	rt := api.NewRuntime(settings)
	result, err := rt.TerraformCorrelate(cmd.Context(), opts)
	if err != nil {
		_ = EmitError("terraform correlate", err.Error())
		return &ExitError{Code: exitcodes.GeneralError, Err: err}
	}
	if jsonStdout {
		data, err := terraform.ToJSON(result.Result)
		if err != nil {
			_ = EmitError("terraform correlate", err.Error())
			return &ExitError{Code: exitcodes.GeneralError, Err: err}
		}
		data = append(data, '\n')
		if _, err := os.Stdout.Write(data); err != nil {
			return err
		}
	} else {
		path, werr := api.WriteTerraformCorrelationDefault(settings.ReportsDir, result.Result)
		if werr == nil {
			_ = EmitOK("terraform correlate", path)
		} else {
			_ = EmitOK("terraform correlate", result.Result.ExitHint)
		}
	}
	if result.ExitCode != exitcodes.Success {
		return &ExitError{Code: result.ExitCode, Err: nil}
	}
	return nil
}
