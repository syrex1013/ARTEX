package report

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"
)

const htmlTplSrc = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root{--c-bg:#fff;--c-text:#1a1a1a;--c-muted:#666;--c-border:#e2e8f0;--c-critical:#dc2626;--c-high:#ea580c;--c-medium:#d97706;--c-low:#65a30d;--c-info:#3b82f6;--c-code:#f8fafc}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:14px;line-height:1.6;color:var(--c-text);background:var(--c-bg);max-width:900px;margin:0 auto;padding:40px 24px}
h1{font-size:2rem;font-weight:700;margin-bottom:.5rem}
h2{font-size:1.4rem;font-weight:600;margin:2rem 0 1rem;padding-bottom:.4rem;border-bottom:2px solid var(--c-border)}
h3{font-size:1.1rem;font-weight:600;margin:1.5rem 0 .5rem}
p{margin:.5rem 0}
table{width:100%;border-collapse:collapse;margin:1rem 0}
th,td{text-align:left;padding:8px 12px;border-bottom:1px solid var(--c-border)}
th{font-weight:600;background:#f8fafc}
pre{background:var(--c-code);padding:12px;border-radius:6px;overflow:auto;font-size:12px;white-space:pre-wrap;word-break:break-all;border:1px solid var(--c-border);margin:.5rem 0}
.meta{color:var(--c-muted);font-size:.85rem;margin-bottom:2rem}
.meta span{margin-right:1.5rem}
.finding{margin:2rem 0;padding:1.5rem;border:1px solid var(--c-border);border-radius:8px;page-break-inside:avoid}
.finding-header{display:flex;align-items:center;gap:1rem;margin-bottom:1rem}
.badge{display:inline-block;padding:3px 10px;border-radius:4px;font-size:.75rem;font-weight:600;text-transform:uppercase}
.sev-critical{background:#fee2e2;color:var(--c-critical)}
.sev-high{background:#ffedd5;color:var(--c-high)}
.sev-medium{background:#fef3c7;color:var(--c-medium)}
.sev-low{background:#ecfccb;color:var(--c-low)}
.sev-info{background:#eff6ff;color:var(--c-info)}
.meta-row{display:flex;flex-wrap:wrap;gap:.5rem;margin:.5rem 0;font-size:.85rem}
.meta-tag{background:#f1f5f9;padding:2px 8px;border-radius:4px;color:var(--c-muted)}
.section-label{font-weight:600;margin-top:1rem;margin-bottom:.25rem;color:var(--c-muted);font-size:.85rem;text-transform:uppercase;letter-spacing:.05em}
.attack-step{display:flex;gap:1rem;padding:.5rem 0;border-bottom:1px solid var(--c-border)}
.step-num{font-weight:700;color:var(--c-muted);min-width:2rem}
footer{margin-top:3rem;padding-top:1rem;border-top:1px solid var(--c-border);font-size:.8rem;color:var(--c-muted)}
@media print{body{padding:0}@page{margin:1.5cm}.finding{page-break-inside:avoid}}
</style>
</head>
<body>
<h1>{{.Title}}</h1>
<div class="meta">
  {{if .Branding.Client}}<span><strong>Client:</strong> {{.Branding.Client}}</span>{{end}}
  {{if .Branding.Author}}<span><strong>Author:</strong> {{.Branding.Author}}</span>{{end}}
  <span><strong>Generated:</strong> {{.GeneratedAt.Format "2006-01-02 15:04 UTC"}}</span>
</div>

{{if .ExecSummary}}<h2>Executive Summary</h2><p>{{.ExecSummary}}</p>{{end}}

{{with includedSorted .Findings}}
<h2>Findings Summary</h2>
<table>
  <thead><tr><th>#</th><th>Severity</th><th>Title</th><th>Category</th><th>CVSS</th></tr></thead>
  <tbody>
  {{range $i, $f := .}}
  <tr>
    <td>{{inc $i}}</td>
    <td><span class="badge {{cvssClass $f.Severity}}">{{$f.Severity}}</span></td>
    <td>{{$f.Name}}</td>
    <td>{{$f.VulnClass}}</td>
    <td>{{fmtScore $f}}</td>
  </tr>
  {{end}}
  </tbody>
</table>

<h2>Detailed Findings</h2>
{{range $i, $f := .}}
<div class="finding">
  <div class="finding-header">
    <span class="badge {{cvssClass $f.Severity}}">{{$f.Severity}}</span>
    <h3 style="margin:0">{{inc $i}}. {{$f.Name}}</h3>
  </div>
  <div class="meta-row">
    <span class="meta-tag">{{$f.VulnClass}}</span>
    {{range $f.CWE}}<span class="meta-tag">{{.}}</span>{{end}}
    {{range $f.OWASP}}<span class="meta-tag">{{.}}</span>{{end}}
    {{range $f.Attack}}<span class="meta-tag">{{.}}</span>{{end}}
    {{if $f.CVSSVector}}<span class="meta-tag">CVSS {{fmtScore $f}}</span>{{end}}
  </div>
  <p class="section-label">Description</p>
  <p>{{effective $f}}</p>
  {{if $f.Evidence}}<p class="section-label">Evidence</p><pre>{{$f.Evidence}}</pre>{{end}}
  {{if $f.Remediation}}<p class="section-label">Remediation</p><p>{{$f.Remediation}}</p>{{end}}
  {{if $f.CVSSVector}}<p class="section-label">CVSS Vector</p><pre>{{$f.CVSSVector}}</pre>{{end}}
</div>
{{end}}
{{end}}

{{if .AttackPaths}}
<h2>Attack Paths</h2>
{{range .AttackPaths}}
<h3>{{.Name}}</h3>
<p>{{.Description}}</p>
{{range .Steps}}
<div class="attack-step">
  <span class="step-num">{{.Position}}.</span>
  <div>
    <strong>{{.Label}}</strong>
    {{if .Tactic}}<span class="meta-tag" style="font-size:.75rem;margin-left:.5rem">{{.Tactic}}</span>{{end}}
    {{if .Technique}}<span class="meta-tag" style="font-size:.75rem">{{.Technique}}</span>{{end}}
    {{if .Note}}<br><small>{{.Note}}</small>{{end}}
  </div>
</div>
{{end}}
{{end}}
{{end}}

{{if .Branding.Footer}}<footer>{{.Branding.Footer}}</footer>{{end}}
</body>
</html>`

func htmlFuncs() template.FuncMap {
	return template.FuncMap{
		"join": strings.Join,
		"cvssClass": func(sev string) string {
			switch sev {
			case "critical":
				return "sev-critical"
			case "high":
				return "sev-high"
			case "medium":
				return "sev-medium"
			case "low":
				return "sev-low"
			default:
				return "sev-info"
			}
		},
		"inc": func(i int) int { return i + 1 },
		"effective": func(f FindingEntry) string {
			if f.SummaryOverride != "" {
				return f.SummaryOverride
			}
			return f.Summary
		},
		"fmtScore": func(f FindingEntry) string {
			if f.CVSSScore == nil {
				return "—"
			}
			return fmt.Sprintf("%.1f", *f.CVSSScore)
		},
		"includedSorted": func(fs []FindingEntry) []FindingEntry {
			inc := includedFindings(fs)
			sort.Slice(inc, func(i, j int) bool {
				return mdSevRank[inc[i].Severity] < mdSevRank[inc[j].Severity]
			})
			return inc
		},
	}
}

// RenderHTML renders a Document as a self-contained HTML page (print-to-PDF ready).
func RenderHTML(doc Document) (string, error) {
	tpl, err := template.New("report").Funcs(htmlFuncs()).Parse(htmlTplSrc)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}
