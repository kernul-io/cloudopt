package report

import (
	"fmt"
	"html"
	"strings"
)

// RenderHTML produces a self-contained HTML document with no external assets.
func RenderHTML(doc *Document) (string, error) {
	if err := ValidateDocument(doc); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>")
	b.WriteString(html.EscapeString(doc.Executive.Headline))
	b.WriteString("</title>\n<style>\n")
	b.WriteString(htmlStyles)
	b.WriteString("</style>\n</head>\n<body>\n")

	b.WriteString("<header><h1>Cloud Optimization Report</h1>")
	if doc.Customer.CustomerName != "" {
		b.WriteString("<p class=\"meta\"><strong>Customer:</strong> ")
		b.WriteString(html.EscapeString(doc.Customer.CustomerName))
		b.WriteString("</p>")
	}
	if doc.Customer.ProjectName != "" {
		b.WriteString("<p class=\"meta\"><strong>Project:</strong> ")
		b.WriteString(html.EscapeString(doc.Customer.ProjectName))
		b.WriteString("</p>")
	}
	b.WriteString("<p class=\"meta\"><strong>Generated:</strong> ")
	b.WriteString(html.EscapeString(doc.GeneratedAt.Format("2006-01-02 15:04:05 UTC")))
	b.WriteString("</p></header>\n")

	b.WriteString("<section><h2>Executive summary</h2><p>")
	b.WriteString(html.EscapeString(doc.Executive.SummaryText))
	b.WriteString("</p><ul>")
	fmt.Fprintf(&b, "<li>Findings: %d</li>", doc.Executive.FindingCount)
	fmt.Fprintf(&b, "<li>Checks failed: %d</li>", doc.Executive.ChecksFailed)
	fmt.Fprintf(&b, "<li>Checks passed: %d</li>", doc.Executive.ChecksPassed)
	fmt.Fprintf(&b, "<li>Suppressed: %d</li>", doc.Executive.ChecksSuppressed)
	fmt.Fprintf(&b, "<li>Not evaluated: %d</li>", doc.Executive.ChecksNotEvaluated)
	b.WriteString("</ul></section>\n")

	b.WriteString("<section><h2>Scope</h2><ul>")
	for _, p := range doc.Scope.Providers {
		b.WriteString("<li>Provider: ")
		b.WriteString(html.EscapeString(p))
		b.WriteString("</li>")
	}
	for _, a := range doc.Scope.Accounts {
		b.WriteString("<li>Account: ")
		b.WriteString(html.EscapeString(a.DisplayName))
		b.WriteString(" (")
		b.WriteString(html.EscapeString(a.Provider))
		b.WriteString(")</li>")
	}
	for _, r := range doc.Scope.Regions {
		b.WriteString("<li>Region: ")
		b.WriteString(html.EscapeString(r))
		b.WriteString("</li>")
	}
	b.WriteString("<li>Observation: ")
	b.WriteString(html.EscapeString(doc.Scope.ObservationStart))
	b.WriteString(" — ")
	b.WriteString(html.EscapeString(doc.Scope.ObservationEnd))
	b.WriteString("</li></ul>")
	if len(doc.Scope.DataQualityNotes) > 0 {
		b.WriteString("<h3>Data quality warnings</h3><ul>")
		for _, w := range doc.Scope.DataQualityNotes {
			b.WriteString("<li>")
			b.WriteString(html.EscapeString(w))
			b.WriteString("</li>")
		}
		b.WriteString("</ul>")
	}
	b.WriteString("</section>\n")

	b.WriteString("<section><h2>Cost summary</h2><p><em>")
	b.WriteString(html.EscapeString(string(doc.Costs.Kind)))
	b.WriteString(" — ")
	b.WriteString(html.EscapeString(doc.Costs.PeriodNote))
	b.WriteString("</em></p><ul>")
	for _, c := range doc.Costs.ByCurrency {
		b.WriteString("<li>")
		b.WriteString(html.EscapeString(fmt.Sprintf("%.2f %s", c.AmountMajor, c.Currency)))
		b.WriteString(": ")
		b.WriteString(html.EscapeString(c.Note))
		b.WriteString("</li>")
	}
	b.WriteString("</ul></section>\n")

	b.WriteString("<section><h2>Potential savings (estimates only)</h2><p>")
	b.WriteString(html.EscapeString(doc.Savings.Note))
	b.WriteString("</p>")
	if len(doc.Savings.MonthlyRecurring) == 0 && len(doc.Savings.OneTime) == 0 && len(doc.Savings.CommitmentBased) == 0 {
		b.WriteString("<p>No quantified savings estimates in this analysis run.</p>")
	}
	b.WriteString("</section>\n")

	b.WriteString("<section><h2>Prioritized findings</h2>\n")
	for _, f := range doc.Findings {
		b.WriteString("<article class=\"finding\"><h3>")
		b.WriteString(html.EscapeString(f.Title))
		b.WriteString(" <span class=\"sev\">")
		b.WriteString(html.EscapeString(f.Severity))
		b.WriteString("</span></h3><p>")
		b.WriteString(html.EscapeString(f.Description))
		b.WriteString("</p><p><strong>Confidence:</strong> ")
		b.WriteString(html.EscapeString(fmt.Sprintf("%.0f%%", f.Confidence*100)))
		b.WriteString("</p>")
		if len(f.Resources) > 0 {
			b.WriteString("<p><strong>Resources:</strong> ")
			var names []string
			for _, r := range f.Resources {
				names = append(names, r.Alias)
			}
			b.WriteString(html.EscapeString(strings.Join(names, ", ")))
			b.WriteString("</p>")
		}
		if len(f.Assumptions) > 0 {
			b.WriteString("<p><strong>Assumptions:</strong></p><ul>")
			for _, a := range f.Assumptions {
				b.WriteString("<li>")
				b.WriteString(html.EscapeString(a))
				b.WriteString("</li>")
			}
			b.WriteString("</ul>")
		}
		if len(f.Evidence) > 0 {
			b.WriteString("<p><strong>Evidence:</strong></p><ul>")
			for _, e := range f.Evidence {
				b.WriteString("<li>[")
				b.WriteString(html.EscapeString(string(e.KindTag)))
				b.WriteString("] ")
				b.WriteString(html.EscapeString(e.Summary))
				if e.Missing {
					b.WriteString(" (missing)")
				}
				b.WriteString("</li>")
			}
			b.WriteString("</ul>")
		}
		b.WriteString("<p><strong>Remediation</strong> (")
		b.WriteString(html.EscapeString(string(f.Remediation.Kind)))
		b.WriteString("): ")
		b.WriteString(html.EscapeString(f.Remediation.Summary))
		b.WriteString("</p><p><strong>Rollback:</strong> ")
		b.WriteString(html.EscapeString(f.Remediation.Rollback))
		b.WriteString("</p></article>\n")
	}
	b.WriteString("</section>\n")

	b.WriteString("<section><h2>Appendix</h2>")
	writeAppendixList(&b, "Suppressed checks", doc.Appendix.Suppressed)
	writeAppendixList(&b, "Not evaluated checks", doc.Appendix.NotEvaluated)
	b.WriteString("</section>\n")

	b.WriteString("<footer><h2>Metadata</h2><ul>")
	b.WriteString("<li>Analyzer version: ")
	b.WriteString(html.EscapeString(doc.Analyzer.Version))
	b.WriteString("</li><li>Snapshot ID: ")
	b.WriteString(html.EscapeString(doc.Analyzer.SnapshotID))
	b.WriteString("</li><li>Analysis run ID: ")
	b.WriteString(html.EscapeString(doc.Analyzer.AnalysisRunID))
	b.WriteString("</li><li>Rule set version: ")
	b.WriteString(html.EscapeString(doc.Analyzer.RuleSetVersion))
	b.WriteString("</li><li>Report schema: ")
	b.WriteString(html.EscapeString(doc.SchemaVersion))
	b.WriteString("</li></ul><p class=\"disclaimer\">")
	b.WriteString(html.EscapeString(doc.Disclaimer))
	b.WriteString("</p></footer>\n</body></html>\n")
	return b.String(), nil
}

func writeAppendixList(b *strings.Builder, title string, items []RuleOutcome) {
	if len(items) == 0 {
		return
	}
	b.WriteString("<h3>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</h3><ul>")
	for _, it := range items {
		b.WriteString("<li><code>")
		b.WriteString(html.EscapeString(it.RuleID))
		b.WriteString("</code>: ")
		b.WriteString(html.EscapeString(it.Message))
		b.WriteString("</li>")
	}
	b.WriteString("</ul>")
}

const htmlStyles = `
body{font-family:system-ui,sans-serif;line-height:1.5;max-width:960px;margin:2rem auto;padding:0 1rem;color:#1a1a1a}
header{border-bottom:2px solid #333;padding-bottom:1rem;margin-bottom:1.5rem}
h1{font-size:1.75rem;margin:0 0 .5rem}
h2{margin-top:2rem;border-bottom:1px solid #ccc;padding-bottom:.25rem}
h3{font-size:1.1rem}
.meta{color:#444;margin:.25rem 0}
.finding{border:1px solid #ddd;border-radius:6px;padding:1rem;margin:1rem 0;background:#fafafa}
.sev{font-size:.85rem;background:#eee;padding:.1rem .4rem;border-radius:4px}
.disclaimer{font-size:.9rem;color:#555;border-top:1px solid #ccc;padding-top:1rem;margin-top:2rem}
code{font-size:.85rem}
`
