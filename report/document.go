package report

import "time"

// Branding holds client/author customisation for a report.
type Branding struct {
	Client     string `json:"client"`
	Author     string `json:"author"`
	LogoDataURL string `json:"logo_data_url,omitempty"` // ≤256 KB data: URI
	Footer     string `json:"footer"`
}

// FindingEntry is one finding's data inside a Document.
type FindingEntry struct {
	ID              int64    `json:"id"`
	Position        int      `json:"position"`
	VulnClass       string   `json:"vulnclass"`
	Name            string   `json:"name"`
	Severity        string   `json:"severity"`
	Summary         string   `json:"summary"`
	Evidence        string   `json:"evidence"`
	Remediation     string   `json:"remediation"`
	CVSSVector      string   `json:"cvss_vector,omitempty"`
	CVSSScore       *float64 `json:"cvss_score,omitempty"`
	CVSSSeverity    string   `json:"cvss_severity,omitempty"`
	CWE             []string `json:"cwe,omitempty"`
	OWASP           []string `json:"owasp,omitempty"`
	Attack          []string `json:"attack,omitempty"`
	SummaryOverride string   `json:"summary_override,omitempty"`
	Included        bool     `json:"included"`
}

// AttackPathEntry is one attack path summary inside a Document.
type AttackPathEntry struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Steps       []PathStepEntry `json:"steps"`
}

// PathStepEntry is one step in an attack path.
type PathStepEntry struct {
	Position  int    `json:"position"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Tactic    string `json:"tactic,omitempty"`
	Technique string `json:"technique,omitempty"`
	Note      string `json:"note,omitempty"`
}

// Document is the canonical model for all report renderers.
type Document struct {
	Title       string         `json:"title"`
	Template    string         `json:"template"` // "technical" | "executive"
	Branding    Branding       `json:"branding"`
	ExecSummary string         `json:"exec_summary"`
	GeneratedAt time.Time      `json:"generated_at"`
	Findings    []FindingEntry `json:"findings"`
	AttackPaths []AttackPathEntry `json:"attack_paths,omitempty"`
}
