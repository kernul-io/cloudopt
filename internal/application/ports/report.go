package ports

import (
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

// ReportFormat selects export encoding.
type ReportFormat string

const (
	ReportHTML ReportFormat = "html"
	ReportJSON ReportFormat = "json"
)

// ReportOptions configures report generation.
type ReportOptions struct {
	AnalysisRunID     types.AnalysisRunID
	RedactIdentifiers bool
	Format            ReportFormat
	OutputPath        string
	AnalyzerVersion   string
}

// ReportResult describes written report artifacts.
type ReportResult struct {
	DocumentPath  string
	Format        ReportFormat
	AnalysisRunID types.AnalysisRunID
	SnapshotID    types.SnapshotID
}
