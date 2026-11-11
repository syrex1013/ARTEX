package report

import (
	"fmt"
	"sort"
	"strings"
)

var mdSevRank = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4, "": 5}

// RenderMarkdown renders a Document as a Markdown string.
func RenderMarkdown(doc Document) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", nz(doc.Title, "Penetration Test Report"))
	if doc.Branding.Client != "" {
		fmt.Fprintf(&b, "**Client:** %s  \n", doc.Branding.Client)
	}
	if doc.Branding.Author != "" {
		fmt.Fprintf(&b, "**Author:** %s  \n", doc.Branding.Author)
	}
	fmt.Fprintf(&b, "**Generated:** %s\n\n", doc.GeneratedAt.Format("2006-01-02 15:04:05 UTC"))

	if doc.ExecSummary != "" {
		fmt.Fprintf(&b, "## Executive Summary\n\n%s\n\n", doc.ExecSummary)
	}

	included := includedFindings(doc.Findings)
	sort.Slice(included, func(i, j int) bool {
		return mdSevRank[included[i].Severity] < mdSevRank[included[j].Severity]
	})

	fmt.Fprintf(&b, "## Findings Summary\n\n")
	fmt.Fprintf(&b, "| # | Severity | Title | CVSS |\n|---|----------|-------|------|\n")
	for i, f := range included {
		score := "-"
		if f.CVSSScore != nil {
			score = fmt.Sprintf("%.1f", *f.CVSSScore)
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", i+1, f.Severity, f.Name, score)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "## Detailed Findings\n\n")
	for i, f := range included {
		effective := f.Summary
		if f.SummaryOverride != "" {
			effective = f.SummaryOverride
		}
		fmt.Fprintf(&b, "### %d. %s\n\n", i+1, f.Name)
		fmt.Fprintf(&b, "- **Severity:** %s\n", f.Severity)
		fmt.Fprintf(&b, "- **Category:** %s\n", f.VulnClass)
		if f.CVSSVector != "" {
			score := "-"
			if f.CVSSScore != nil {
				score = fmt.Sprintf("%.1f (%s)", *f.CVSSScore, f.CVSSSeverity)
			}
			fmt.Fprintf(&b, "- **CVSS:** %s — %s\n", score, f.CVSSVector)
		}
		if len(f.CWE) > 0 {
			fmt.Fprintf(&b, "- **CWE:** %s\n", strings.Join(f.CWE, ", "))
		}
		if len(f.OWASP) > 0 {
			fmt.Fprintf(&b, "- **OWASP:** %s\n", strings.Join(f.OWASP, ", "))
		}
		if len(f.Attack) > 0 {
			fmt.Fprintf(&b, "- **ATT&CK:** %s\n", strings.Join(f.Attack, ", "))
		}
		fmt.Fprintf(&b, "\n**Description:**\n\n%s\n\n", effective)
		if f.Evidence != "" {
			fmt.Fprintf(&b, "**Evidence:**\n\n```\n%s\n```\n\n", f.Evidence)
		}
		if f.Remediation != "" {
			fmt.Fprintf(&b, "**Remediation:**\n\n%s\n\n", f.Remediation)
		}
		b.WriteString("---\n\n")
	}

	if len(doc.AttackPaths) > 0 {
		fmt.Fprintf(&b, "## Attack Paths\n\n")
		for _, ap := range doc.AttackPaths {
			fmt.Fprintf(&b, "### %s\n\n%s\n\n", ap.Name, ap.Description)
			for _, step := range ap.Steps {
				tactic := ""
				if step.Tactic != "" {
					tactic = fmt.Sprintf(" [%s]", step.Tactic)
				}
				fmt.Fprintf(&b, "%d. **%s**%s — %s\n", step.Position, step.Label, tactic, step.Note)
			}
			b.WriteString("\n")
		}
	}

	if doc.Branding.Footer != "" {
		fmt.Fprintf(&b, "---\n\n%s\n", doc.Branding.Footer)
	}

	return b.String()
}

func includedFindings(fs []FindingEntry) []FindingEntry {
	var out []FindingEntry
	for _, f := range fs {
		if f.Included {
			out = append(out, f)
		}
	}
	return out
}
