package capabilities

import (
	"strings"

	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// ServiceCategory normalizes vendor-specific service names for portfolio summaries.
func ServiceCategory(provider types.Provider, service string) string {
	s := strings.TrimSpace(service)
	if s == "" {
		return "unknown"
	}
	upper := strings.ToUpper(s)
	switch provider {
	case types.ProviderAWS, types.ProviderFixture:
		switch {
		case strings.Contains(upper, "EC2"), strings.Contains(upper, "ELASTIC COMPUTE"):
			return "compute"
		case strings.Contains(upper, "EBS"), strings.Contains(upper, "S3"), strings.Contains(upper, "STORAGE"):
			return "storage"
		case strings.Contains(upper, "RDS"), strings.Contains(upper, "DATABASE"):
			return "database"
		case strings.Contains(upper, "VPC"), strings.Contains(upper, "NAT"), strings.Contains(upper, "NETWORK"):
			return "network"
		default:
			return "other"
		}
	case types.ProviderGCP:
		switch {
		case strings.Contains(upper, "COMPUTE ENGINE"), strings.Contains(upper, "GCE"):
			return "compute"
		case strings.Contains(upper, "PERSISTENT DISK"), strings.Contains(upper, "STORAGE"):
			return "storage"
		case strings.Contains(upper, "CLOUD SQL"):
			return "database"
		case strings.Contains(upper, "NETWORK"), strings.Contains(upper, "NAT"):
			return "network"
		default:
			return "other"
		}
	default:
		return "other"
	}
}
