package report_test

import (
	"context"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kernul-io/cloudopt/internal/adapters/fixture"
	sqliterepository "github.com/kernul-io/cloudopt/internal/adapters/sqlite-repository"
	"github.com/kernul-io/cloudopt/internal/application/api"
	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
	"github.com/kernul-io/cloudopt/internal/application/report"
	"github.com/kernul-io/cloudopt/internal/application/rules"
)

func TestReportFixtureGoldenJSON(t *testing.T) {
	doc := buildFixtureReportDoc(t, false, time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC))
	require.NoError(t, report.ValidateDocument(doc))
	require.Equal(t, report.SchemaVersion, doc.SchemaVersion)
	require.Len(t, doc.Findings, 8)
	require.NotEmpty(t, doc.Disclaimer)
	require.Contains(t, doc.Disclaimer, "not guaranteed")

	data, err := report.ToJSON(doc)
	require.NoError(t, err)
	var round map[string]any
	require.NoError(t, json.Unmarshal(data, &round))
	require.Equal(t, report.SchemaVersion, round["schema_version"])
}

func TestReportDeterministicFindingOrder(t *testing.T) {
	fixed := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	a := buildFixtureReportDoc(t, false, fixed)
	b := buildFixtureReportDoc(t, false, fixed)
	require.Equal(t, len(a.Findings), len(b.Findings))
	for i := range a.Findings {
		require.Equal(t, a.Findings[i].Fingerprint, b.Findings[i].Fingerprint)
		require.Equal(t, a.Findings[i].ID, b.Findings[i].ID)
	}
}

func TestReportHTMLEscapesCustomerMetadata(t *testing.T) {
	doc := buildFixtureReportDoc(t, false, time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC))
	doc.Customer.CustomerName = `<script>alert("x")</script>`
	doc.Executive.Headline = `"><img onerror=alert(1)>`
	out, err := report.RenderHTML(doc)
	require.NoError(t, err)
	require.NotContains(t, out, "<script>")
	require.Contains(t, out, html.EscapeString(doc.Customer.CustomerName))
	require.NotContains(t, out, "<img onerror")
}

func TestReportRedactionAliases(t *testing.T) {
	doc := buildFixtureReportDoc(t, true, time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC))
	require.Equal(t, "Account-1", doc.Scope.Accounts[0].DisplayName)
	require.NotContains(t, doc.Findings[0].Resources[0].Alias, "forgotten")
	for _, f := range doc.Findings {
		for _, r := range f.Resources {
			require.Contains(t, r.Alias, "Resource-")
		}
	}
}

func TestReportMissingEvidencePlaceholder(t *testing.T) {
	doc := buildFixtureReportDoc(t, false, time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC))
	if len(doc.Findings) == 0 {
		t.Fatal("expected findings")
	}
	doc.Findings[0].Evidence = []report.EvidenceEntry{{
		Kind:    "missing",
		Summary: "Evidence record not found in analysis run",
		Missing: true,
		KindTag: report.KindDerived,
	}}
	out, err := report.RenderHTML(doc)
	require.NoError(t, err)
	require.Contains(t, out, "missing")
}

func TestOfflineDemoFlow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	reportsDir := filepath.Join(dir, "reports")
	configDir := filepath.Join(dir, "config")
	require.NoError(t, os.MkdirAll(dataDir, 0o750))
	require.NoError(t, os.MkdirAll(configDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "report.yaml"), []byte(`
customer_name: Demo Startup
project_name: Offline Slice
`), 0o600))

	repo := openTestRepoAt(t, filepath.Join(dataDir, "cloudopt.db"))
	importer := fixture.NewImporter(repo)
	fixturePath := filepath.Join("..", "..", "..", "testdata", "fixtures", "sample.yaml")
	snapID, err := importer.Import(ctx, fixturePath)
	require.NoError(t, err)

	svc := &api.AnalyzeService{Repo: repo}
	analyzeOut, err := svc.Analyze(ctx, api.AnalyzeSettings{ConfigDir: configDir}, ports.AnalyzeOptions{Persist: true})
	require.NoError(t, err)
	require.Equal(t, snapID, analyzeOut.SnapshotID)

	reportSvc := &api.ReportService{Repo: repo, ReportsDir: reportsDir}
	htmlPath := filepath.Join(reportsDir, "demo.html")
	result, err := reportSvc.Generate(ctx, api.AnalyzeSettings{ConfigDir: configDir}, ports.ReportOptions{
		Format:          ports.ReportHTML,
		OutputPath:      htmlPath,
		AnalyzerVersion: "test-1.0",
	})
	require.NoError(t, err)
	require.Equal(t, htmlPath, result.DocumentPath)
	body, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	require.Contains(t, string(body), "Offline Slice")
	require.Contains(t, string(body), "read-only analysis")
}

func buildFixtureReportDoc(t *testing.T, redact bool, generatedAt time.Time) *report.Document {
	t.Helper()
	ctx := context.Background()
	repo := openTestRepo(t)
	importer := fixture.NewImporter(repo)
	fixturePath := filepath.Join("..", "..", "..", "testdata", "fixtures", "sample.yaml")
	_, err := importer.Import(ctx, fixturePath)
	require.NoError(t, err)

	svc := &api.AnalyzeService{Repo: repo}
	out, err := svc.Analyze(ctx, api.AnalyzeSettings{}, ports.AnalyzeOptions{Persist: true})
	require.NoError(t, err)
	run, err := repo.GetAnalysisRun(ctx, out.AnalysisRunID)
	require.NoError(t, err)
	snap, err := repo.GetSnapshot(ctx, out.SnapshotID)
	require.NoError(t, err)

	reg := rules.DefaultRegistry(nil)
	manifest, err := rules.LoadManifest("", reg)
	require.NoError(t, err)
	engineOut, err := rules.Engine{}.Analyze(rules.AnalyzeInput{
		Snapshot:       snap,
		Manifest:       manifest,
		Registry:       reg,
		Suppressions:   rules.NewSuppressionIndex(nil, snap.StartedAt.Time),
		PricingCatalog: nil,
	})
	require.NoError(t, err)

	doc, err := report.Build(report.BuildInput{
		AnalyzerVersion: "test",
		GeneratedAt:     generatedAt,
		Metadata: report.Metadata{
			CustomerName: "Test Co",
			ProjectName:  "Fixture",
		},
		Snapshot:   snap,
		Run:        run,
		Executions: engineOut.Executions,
		Redact:     redact,
	})
	require.NoError(t, err)
	return doc
}

func openTestRepo(t *testing.T) *sqliterepository.Repository {
	t.Helper()
	return openTestRepoAt(t, ":memory:")
}

func openTestRepoAt(t *testing.T, path string) *sqliterepository.Repository {
	t.Helper()
	db, err := sqliterepository.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := sqliterepository.NewRepository(db)
	require.NoError(t, repo.Migrate(context.Background()))
	return repo
}

func TestGetLatestAnalysisRun(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepo(t)
	_, err := repo.GetLatestAnalysisRun(ctx, "")
	require.Error(t, err)

	snap := &domain.CollectionSnapshot{
		ID:            "snap-a",
		AccountID:     "acct-1",
		Provider:      types.Provider("fixture"),
		Status:        domain.SnapshotComplete,
		SchemaVersion: 1,
		StartedAt:     mustTS(t, "2026-01-15T12:00:00Z"),
		CompletedAt:   ptrTS(mustTS(t, "2026-01-15T12:00:00Z")),
		Account: domain.Account{
			ID:                "acct-1",
			Provider:          "fixture",
			ProviderAccountID: "1",
			DisplayName:       "A",
			DefaultCurrency:   "USD",
		},
	}
	require.NoError(t, repo.SaveSnapshot(ctx, snap))

	completed1 := mustTS(t, "2026-01-15T12:00:00Z")
	completed2 := mustTS(t, "2026-01-16T12:00:00Z")
	run1 := &domain.AnalysisRun{
		ID: "run-1", SnapshotID: snap.ID, Status: domain.AnalysisComplete,
		RuleSetVersion: "v1", StartedAt: snap.StartedAt, CompletedAt: &completed1,
	}
	run2 := &domain.AnalysisRun{
		ID: "run-2", SnapshotID: snap.ID, Status: domain.AnalysisComplete,
		RuleSetVersion: "v1", StartedAt: snap.StartedAt, CompletedAt: &completed2,
	}
	require.NoError(t, repo.SaveAnalysisRun(ctx, run1))
	require.NoError(t, repo.SaveAnalysisRun(ctx, run2))

	latest, err := repo.GetLatestAnalysisRun(ctx, "")
	require.NoError(t, err)
	require.Equal(t, types.AnalysisRunID("run-2"), latest.ID)

	bySnap, err := repo.GetLatestAnalysisRun(ctx, snap.ID)
	require.NoError(t, err)
	require.Equal(t, types.AnalysisRunID("run-2"), bySnap.ID)
}

func mustTS(t *testing.T, s string) types.Timestamp {
	t.Helper()
	ts, err := types.ParseTimestamp(s)
	require.NoError(t, err)
	return ts
}

func ptrTS(ts types.Timestamp) *types.Timestamp {
	return &ts
}
