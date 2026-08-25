package terraform

import (
	"github.com/kernul-io/cloudopt/internal/domain"
)

// patchForFinding returns an optional patch hint only for high-confidence, unambiguous links.
func patchForFinding(f domain.Finding, links []CorrelationLink) *PatchSuggestion {
	if len(links) != 1 {
		return nil
	}
	link := links[0]
	if link.Ambiguous || link.Confidence != ConfidenceHigh {
		return nil
	}
	switch f.RuleID {
	case "ebs-unattached-volume", "gcp-unattached-disk":
		return &PatchSuggestion{
			TFAddress:      link.TFAddress,
			Attribute:      volumeSizeAttribute(link),
			SuggestedValue: "destroy resource or attach to instance — review manually",
			Confidence:     ConfidenceMedium,
			RequiresReview: true,
		}
	case "ec2-idle-instance", "gcp-idle-instance":
		attr, val := rightsizingHint(link)
		if attr == "" {
			return nil
		}
		return &PatchSuggestion{
			TFAddress:      link.TFAddress,
			Attribute:      attr,
			SuggestedValue: val,
			Confidence:     ConfidenceMedium,
			RequiresReview: true,
		}
	default:
		return nil
	}
}

func volumeSizeAttribute(link CorrelationLink) string {
	for _, a := range link.Attributes {
		if a.Name == "size" || a.Name == "volume_size" {
			return a.Name
		}
	}
	return "size"
}

func rightsizingHint(link CorrelationLink) (attr, suggested string) {
	for _, a := range link.Attributes {
		switch a.Name {
		case "instance_type":
			return "instance_type", "downsize after metrics review — set explicitly in Terraform"
		case "machine_type":
			return "machine_type", "downsize after metrics review — set explicitly in Terraform"
		case "size_slug":
			return "size_slug", "select smaller slug after metrics review"
		}
	}
	return "", ""
}
