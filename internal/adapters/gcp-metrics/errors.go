package gcpmetrics

import "fmt"

const collectorSource = "gcp-metrics/cloud-monitoring"

func errWrap(op string, err error) error {
	return fmt.Errorf("gcp metrics %s: %w", op, err)
}
