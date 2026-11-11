package mailer

import (
	"bytes"
	"html/template"
	"strings"
	text_template "text/template"
)

// TargetVars holds per-target template variables for phishing emails.
type TargetVars struct {
	Name       string // target full name
	FirstName  string // first name only
	LastName   string
	Email      string
	Department string
	Company    string
	// Campaign-level
	Link         string // main phishing link (tracking + redirect)
	TrackingLink string // 1×1 pixel URL
	InvoiceNo    string
	TicketNo     string
	// Operator-supplied extras
	Extra map[string]string
}

// RenderHTML substitutes variables in an HTML template body using html/template
// (auto-escaping). Falls back to text substitution on parse errors.
func RenderHTML(tpl string, vars TargetVars) (string, error) {
	t, err := template.New("body").Parse(tpl)
	if err != nil {
		return renderText(tpl, vars), nil
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return renderText(tpl, vars), nil
	}
	return buf.String(), nil
}

// RenderText substitutes variables in a plain-text template.
func RenderText(tpl string, vars TargetVars) string {
	return renderText(tpl, vars)
}

func renderText(tpl string, vars TargetVars) string {
	t, err := text_template.New("body").Parse(tpl)
	if err != nil {
		return simpleSub(tpl, vars)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return simpleSub(tpl, vars)
	}
	return buf.String()
}

// simpleSub is a {{.Field}} → value replacer for broken templates.
func simpleSub(tpl string, vars TargetVars) string {
	r := strings.NewReplacer(
		"{{.Name}}", vars.Name,
		"{{.FirstName}}", vars.FirstName,
		"{{.LastName}}", vars.LastName,
		"{{.Email}}", vars.Email,
		"{{.Department}}", vars.Department,
		"{{.Company}}", vars.Company,
		"{{.Link}}", vars.Link,
		"{{.TrackingLink}}", vars.TrackingLink,
		"{{.InvoiceNo}}", vars.InvoiceNo,
		"{{.TicketNo}}", vars.TicketNo,
	)
	return r.Replace(tpl)
}

// FirstName extracts the first word of a full name.
func FirstName(full string) string {
	parts := strings.Fields(full)
	if len(parts) == 0 {
		return full
	}
	return parts[0]
}

// LastName extracts everything after the first word.
func LastName(full string) string {
	parts := strings.Fields(full)
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[1:], " ")
}
