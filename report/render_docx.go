// Package report - pure-Go DOCX renderer using archive/zip (OOXML).
// Produces a minimal but valid .docx: headings, paragraphs, tables, bold/code runs.
package report

import (
	"archive/zip"
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// RenderDOCX renders a Document as a .docx byte slice.
func RenderDOCX(doc Document) ([]byte, error) {
	body := buildDocxBody(doc)
	return packDocx(body)
}

// ---- OOXML body builder ----

func buildDocxBody(doc Document) string {
	var b strings.Builder

	heading(&b, 1, nz(doc.Title, "Penetration Test Report"))
	if doc.Branding.Client != "" {
		para(&b, "Client: "+doc.Branding.Client)
	}
	if doc.Branding.Author != "" {
		para(&b, "Author: "+doc.Branding.Author)
	}
	para(&b, "Generated: "+doc.GeneratedAt.Format("2006-01-02 15:04 UTC"))

	if doc.ExecSummary != "" {
		heading(&b, 2, "Executive Summary")
		para(&b, doc.ExecSummary)
	}

	included := includedFindings(doc.Findings)
	sort.Slice(included, func(i, j int) bool {
		return mdSevRank[included[i].Severity] < mdSevRank[included[j].Severity]
	})

	if len(included) > 0 {
		heading(&b, 2, "Findings Summary")
		rows := [][]string{{"#", "Severity", "Title", "CVSS"}}
		for i, f := range included {
			score := "—"
			if f.CVSSScore != nil {
				score = fmt.Sprintf("%.1f", *f.CVSSScore)
			}
			rows = append(rows, []string{fmt.Sprintf("%d", i+1), f.Severity, f.Name, score})
		}
		table(&b, rows)

		heading(&b, 2, "Detailed Findings")
		for i, f := range included {
			effective := f.Summary
			if f.SummaryOverride != "" {
				effective = f.SummaryOverride
			}
			heading(&b, 3, fmt.Sprintf("%d. %s", i+1, f.Name))
			meta := []string{"Severity: " + f.Severity, "Category: " + f.VulnClass}
			if f.CVSSVector != "" {
				score := "—"
				if f.CVSSScore != nil {
					score = fmt.Sprintf("%.1f", *f.CVSSScore)
				}
				meta = append(meta, "CVSS: "+score+" — "+f.CVSSVector)
			}
			if len(f.CWE) > 0 {
				meta = append(meta, "CWE: "+strings.Join(f.CWE, ", "))
			}
			if len(f.OWASP) > 0 {
				meta = append(meta, "OWASP: "+strings.Join(f.OWASP, ", "))
			}
			if len(f.Attack) > 0 {
				meta = append(meta, "ATT&CK: "+strings.Join(f.Attack, ", "))
			}
			for _, m := range meta {
				para(&b, m)
			}
			boldPara(&b, "Description")
			para(&b, effective)
			if f.Evidence != "" {
				boldPara(&b, "Evidence")
				codePara(&b, f.Evidence)
			}
			if f.Remediation != "" {
				boldPara(&b, "Remediation")
				para(&b, f.Remediation)
			}
		}
	}

	if len(doc.AttackPaths) > 0 {
		heading(&b, 2, "Attack Paths")
		for _, ap := range doc.AttackPaths {
			heading(&b, 3, ap.Name)
			if ap.Description != "" {
				para(&b, ap.Description)
			}
			for _, step := range ap.Steps {
				line := fmt.Sprintf("%d. %s", step.Position, step.Label)
				if step.Tactic != "" {
					line += " [" + step.Tactic + "]"
				}
				if step.Note != "" {
					line += " — " + step.Note
				}
				para(&b, line)
			}
		}
	}

	if doc.Branding.Footer != "" {
		para(&b, "—")
		para(&b, doc.Branding.Footer)
	}

	return b.String()
}

// ---- OOXML helpers ----

func xmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func heading(b *strings.Builder, level int, text string) {
	style := fmt.Sprintf("Heading%d", level)
	fmt.Fprintf(b, `<w:p><w:pPr><w:pStyle w:val="%s"/></w:pPr><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, style, xmlEsc(text))
}

func para(b *strings.Builder, text string) {
	fmt.Fprintf(b, `<w:p><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, xmlEsc(text))
}

func boldPara(b *strings.Builder, text string) {
	fmt.Fprintf(b, `<w:p><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, xmlEsc(text))
}

func codePara(b *strings.Builder, text string) {
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(b, `<w:p><w:pPr><w:pStyle w:val="Code"/></w:pPr><w:r><w:rPr><w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/><w:sz w:val="18"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, xmlEsc(line))
	}
}

func table(b *strings.Builder, rows [][]string) {
	b.WriteString(`<w:tbl><w:tblPr><w:tblStyle w:val="TableGrid"/><w:tblW w:w="0" w:type="auto"/></w:tblPr>`)
	for i, row := range rows {
		b.WriteString(`<w:tr>`)
		for _, cell := range row {
			bold := i == 0
			if bold {
				fmt.Fprintf(b, `<w:tc><w:p><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p></w:tc>`, xmlEsc(cell))
			} else {
				fmt.Fprintf(b, `<w:tc><w:p><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p></w:tc>`, xmlEsc(cell))
			}
		}
		b.WriteString(`</w:tr>`)
	}
	b.WriteString(`</w:tbl>`)
}

// ---- zip / OOXML packager ----

func packDocx(body string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	files := map[string]string{
		"[Content_Types].xml": contentTypesXML,
		"_rels/.rels":         relsXML,
		"word/_rels/document.xml.rels": wordRelsXML,
		"word/document.xml":   fmt.Sprintf(documentXMLTpl, body),
		"word/styles.xml":     stylesXML,
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := fmt.Fprint(w, content); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), zw.Close()
}

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
</Types>`

const relsXML = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const wordRelsXML = `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const documentXMLTpl = `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:wpc="http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas"
  xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
  xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<w:body>%s<w:sectPr/></w:body>
</w:document>`

const stylesXML = `<?xml version="1.0" encoding="UTF-8"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
  <w:style w:type="paragraph" w:styleId="Heading1">
    <w:name w:val="heading 1"/>
    <w:pPr><w:numPr/></w:pPr>
    <w:rPr><w:b/><w:sz w:val="40"/><w:color w:val="1a1a1a"/></w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading2">
    <w:name w:val="heading 2"/>
    <w:rPr><w:b/><w:sz w:val="32"/><w:color w:val="2d3748"/></w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading3">
    <w:name w:val="heading 3"/>
    <w:rPr><w:b/><w:sz w:val="26"/><w:color w:val="4a5568"/></w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Code">
    <w:name w:val="Code"/>
    <w:pPr><w:shd w:val="clear" w:color="auto" w:fill="F7FAFC"/></w:pPr>
    <w:rPr><w:rFonts w:ascii="Courier New" w:hAnsi="Courier New"/><w:sz w:val="18"/></w:rPr>
  </w:style>
  <w:style w:type="table" w:styleId="TableGrid">
    <w:name w:val="Table Grid"/>
    <w:tblPr><w:tblBorders>
      <w:top w:val="single" w:sz="4"/><w:left w:val="single" w:sz="4"/>
      <w:bottom w:val="single" w:sz="4"/><w:right w:val="single" w:sz="4"/>
      <w:insideH w:val="single" w:sz="4"/><w:insideV w:val="single" w:sz="4"/>
    </w:tblBorders></w:tblPr>
  </w:style>
</w:styles>`
