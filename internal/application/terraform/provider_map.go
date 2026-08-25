package terraform

import "github.com/kernul-io/cloudopt/internal/domain/types"

// CloudProviderFromTF maps a Terraform provider source address to the canonical cloud provider.
func CloudProviderFromTF(providerType, providerAlias string) types.Provider {
	switch {
	case contains(providerType, "hashicorp/aws"), contains(providerAlias, "aws"):
		return types.ProviderAWS
	case contains(providerType, "hashicorp/google"), contains(providerType, "hashicorp/google-beta"):
		return types.ProviderGCP
	case contains(providerType, "hashicorp/azurerm"), contains(providerType, "azure/azapi"):
		return types.ProviderAzure
	case contains(providerType, "digitalocean/digitalocean"):
		return types.ProviderDigitalOcean
	default:
		return ""
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// providerIDAttribute returns the Terraform attribute name holding the cloud provider resource ID.
func providerIDAttribute(tfType string) string {
	switch tfType {
	case "google_compute_instance", "google_compute_disk":
		return "id"
	case "azurerm_linux_virtual_machine", "azurerm_windows_virtual_machine",
		"azurerm_managed_disk", "azurerm_virtual_network":
		return "id"
	case "azurerm_resource_group":
		return "id"
	default:
		return "id"
	}
}

// tagFields returns attribute keys used for tag/label correlation per resource type.
func tagFields(tfType string) []string {
	switch tfType {
	case "google_compute_instance", "google_compute_disk":
		return []string{"labels"}
	default:
		return []string{"tags", "tags_all"}
	}
}
