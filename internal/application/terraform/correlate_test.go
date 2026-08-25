package terraform_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/application/terraform"
	"github.com/kernul-io/cloudopt/internal/domain"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func TestCorrelateProviderIDDirect(t *testing.T) {
	tf := []terraform.ManagedResource{{
		Address: "aws_instance.app",
		Type:    "aws_instance",
		Name:    "app",
		Mode:    "managed",
		Values:  map[string]string{"id": "i-0running01", "instance_type": "m5.large"},
	}}
	resources := []domain.Resource{{
		ID:                 "res-i-running",
		ProviderResourceID: "i-0running01",
		Kind:               domain.KindComputeInstance,
	}}
	result := terraform.Correlate(tf, terraform.CorrelateOptions{
		Resources: resources,
		Provider:  types.ProviderAWS,
	})
	require.Len(t, result.Links, 1)
	require.Equal(t, terraform.ConfidenceHigh, result.Links[0].Confidence)
	require.Equal(t, terraform.MethodProviderID, result.Links[0].Method)
	require.False(t, result.Links[0].Ambiguous)
	require.Equal(t, "aws_instance.app", result.Links[0].TFAddress)
}

func TestCorrelateAmbiguousProviderIDNeverHigh(t *testing.T) {
	tf := []terraform.ManagedResource{
		{Address: "aws_instance.a", Type: "aws_instance", Mode: "managed", Values: map[string]string{"id": "i-dup"}},
		{Address: "aws_instance.b", Type: "aws_instance", Mode: "managed", Values: map[string]string{"id": "i-dup"}},
	}
	resources := []domain.Resource{{
		ID: "res-x", ProviderResourceID: "i-dup", Kind: domain.KindComputeInstance,
	}}
	result := terraform.Correlate(tf, terraform.CorrelateOptions{Resources: resources, Provider: types.ProviderAWS})
	require.Len(t, result.Links, 1)
	require.True(t, result.Links[0].Ambiguous)
	require.Equal(t, terraform.ConfidenceAmbiguous, result.Links[0].Confidence)
	require.Len(t, result.Links[0].Candidates, 2)
}

func TestCorrelateUserMapping(t *testing.T) {
	tf := []terraform.ManagedResource{{
		Address: "aws_instance.legacy_name", Type: "aws_instance", Mode: "managed",
		Values: map[string]string{"id": "i-other"},
	}}
	resources := []domain.Resource{{
		ID: "res-renamed", ProviderResourceID: "i-renamed", Kind: domain.KindComputeInstance,
	}}
	result := terraform.Correlate(tf, terraform.CorrelateOptions{
		Resources: resources,
		Provider:  types.ProviderAWS,
		Mappings: []terraform.UserMapping{{
			ResourceID: "res-renamed", TFAddress: "aws_instance.legacy_name",
		}},
	})
	require.Equal(t, terraform.MethodUserMapping, result.Links[0].Method)
	require.Equal(t, terraform.ConfidenceHigh, result.Links[0].Confidence)
}

func TestCorrelateModuleForEachCount(t *testing.T) {
	tf := []terraform.ManagedResource{
		{Address: `module.app.aws_instance.web["prod"]`, Type: "aws_instance", Mode: "managed", Values: map[string]string{"id": "i-foreach01"}},
		{Address: "module.app.aws_instance.worker[0]", Type: "aws_instance", Mode: "managed", Values: map[string]string{"id": "i-count01"}},
		{Address: "module.network.aws_vpc.main", Type: "aws_vpc", Mode: "managed", Values: map[string]string{"id": "vpc-module01"}},
	}
	resources := []domain.Resource{
		{ID: "r1", ProviderResourceID: "i-foreach01", Kind: domain.KindComputeInstance},
		{ID: "r2", ProviderResourceID: "i-count01", Kind: domain.KindComputeInstance},
		{ID: "r3", ProviderResourceID: "vpc-module01", Kind: domain.KindVPC},
	}
	result := terraform.Correlate(tf, terraform.CorrelateOptions{Resources: resources, Provider: types.ProviderAWS})
	require.Len(t, result.Links, 3)
	for _, l := range result.Links {
		require.Equal(t, terraform.ConfidenceHigh, l.Confidence)
		require.Contains(t, l.TFAddress, "module.")
	}
}

func TestCorrelateAllCloudProviders(t *testing.T) {
	tf := []terraform.ManagedResource{
		{Address: "google_compute_instance.app", Type: "google_compute_instance", ProviderType: "registry.terraform.io/hashicorp/google", Mode: "managed", Values: map[string]string{"id": "gce-1"}},
		{Address: "azurerm_linux_virtual_machine.app", Type: "azurerm_linux_virtual_machine", ProviderType: "registry.terraform.io/hashicorp/azurerm", Mode: "managed", Values: map[string]string{"id": "vm-1"}},
		{Address: "digitalocean_droplet.web", Type: "digitalocean_droplet", ProviderType: "registry.terraform.io/digitalocean/digitalocean", Mode: "managed", Values: map[string]string{"id": "512345678"}},
	}
	resources := []domain.Resource{
		{ID: "g", ProviderResourceID: "gce-1", Kind: domain.KindComputeInstance},
		{ID: "a", ProviderResourceID: "vm-1", Kind: domain.KindComputeInstance},
		{ID: "d", ProviderResourceID: "512345678", Kind: domain.KindComputeInstance},
	}
	result := terraform.Correlate(tf, terraform.CorrelateOptions{Resources: resources, Provider: types.ProviderMulti})
	require.Len(t, result.Links, 3)
}

func TestExitCodeAmbiguous(t *testing.T) {
	result := terraform.CorrelationResult{
		Links: []terraform.CorrelationLink{{Ambiguous: true}},
	}
	require.Equal(t, 5, terraform.ExitCodeForResult(result, false))
}

func TestParseModulePath(t *testing.T) {
	mod, _ := terraform.ParseModulePath(`module.app.aws_instance.web["prod"]`)
	require.Equal(t, `module.app`, mod)
}
