package gcpbilling

import "fmt"

const collectorSource = "gcp-billing/bigquery-export"

// ExportNotConfigured indicates live collection without BigQuery export settings.
type ExportNotConfigured struct{}

func (ExportNotConfigured) Error() string {
	return "GCP billing export not configured; set --billing-export-project, --bigquery-dataset, and --bigquery-table or use --offline"
}

func errWrap(op string, err error) error {
	return fmt.Errorf("gcp billing %s: %w", op, err)
}
