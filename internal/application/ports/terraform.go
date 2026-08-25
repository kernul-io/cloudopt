package ports

import (
	"context"

	"github.com/kernul-io/cloudopt/internal/application/terraform"
)

// TerraformInputReader loads parsed Terraform artifacts without invoking the Terraform CLI.
type TerraformInputReader interface {
	LoadStateFile(ctx context.Context, path string) ([]terraform.ManagedResource, error)
	LoadPlanFile(ctx context.Context, path string) ([]terraform.PlanResourceChange, error)
}
