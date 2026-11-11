// Package classify holds the pick lists used to tag findings (CWE, OWASP Top 10 2021, MITRE ATT&CK) and a keyword
// based suggester. The lists are deliberately curated to what web, network and cloud assessments meet, not exhaustive;
// a finding may also carry any other well-formed identifier (see Valid*). Suggestions are always confirmed by a person.
//
// MITRE ATT&CK® is a registered trademark of The MITRE Corporation; technique ids and names are used under MITRE's
// terms (https://attack.mitre.org/resources/terms-of-use/).
package classify

import (
	"regexp"
	"sort"
	"strings"
)

type Entry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Tactic string `json:"tactic,omitempty"` // ATT&CK only
}

var CWE = []Entry{
	{ID: "CWE-16", Name: "Configuration"},
	{ID: "CWE-22", Name: "Improper Limitation of a Pathname to a Restricted Directory (Path Traversal)"},
	{ID: "CWE-77", Name: "Improper Neutralization of Special Elements used in a Command (Command Injection)"},
	{ID: "CWE-78", Name: "Improper Neutralization of Special Elements used in an OS Command (OS Command Injection)"},
	{ID: "CWE-79", Name: "Improper Neutralization of Input During Web Page Generation (Cross-site Scripting)"},
	{ID: "CWE-89", Name: "Improper Neutralization of Special Elements used in an SQL Command (SQL Injection)"},
	{ID: "CWE-90", Name: "Improper Neutralization of Special Elements used in an LDAP Query (LDAP Injection)"},
	{ID: "CWE-94", Name: "Improper Control of Generation of Code (Code Injection)"},
	{ID: "CWE-113", Name: "Improper Neutralization of CRLF Sequences in HTTP Headers (HTTP Response Splitting)"},
	{ID: "CWE-200", Name: "Exposure of Sensitive Information to an Unauthorized Actor"},
	{ID: "CWE-209", Name: "Generation of Error Message Containing Sensitive Information"},
	{ID: "CWE-276", Name: "Incorrect Default Permissions"},
	{ID: "CWE-284", Name: "Improper Access Control"},
	{ID: "CWE-285", Name: "Improper Authorization"},
	{ID: "CWE-287", Name: "Improper Authentication"},
	{ID: "CWE-295", Name: "Improper Certificate Validation"},
	{ID: "CWE-306", Name: "Missing Authentication for Critical Function"},
	{ID: "CWE-307", Name: "Improper Restriction of Excessive Authentication Attempts"},
	{ID: "CWE-312", Name: "Cleartext Storage of Sensitive Information"},
	{ID: "CWE-319", Name: "Cleartext Transmission of Sensitive Information"},
	{ID: "CWE-326", Name: "Inadequate Encryption Strength"},
	{ID: "CWE-327", Name: "Use of a Broken or Risky Cryptographic Algorithm"},
	{ID: "CWE-346", Name: "Origin Validation Error"},
	{ID: "CWE-347", Name: "Improper Verification of Cryptographic Signature"},
	{ID: "CWE-352", Name: "Cross-Site Request Forgery (CSRF)"},
	{ID: "CWE-384", Name: "Session Fixation"},
	{ID: "CWE-400", Name: "Uncontrolled Resource Consumption"},
	{ID: "CWE-434", Name: "Unrestricted Upload of File with Dangerous Type"},
	{ID: "CWE-444", Name: "Inconsistent Interpretation of HTTP Requests (HTTP Request/Response Smuggling)"},
	{ID: "CWE-502", Name: "Deserialization of Untrusted Data"},
	{ID: "CWE-532", Name: "Insertion of Sensitive Information into Log File"},
	{ID: "CWE-538", Name: "Insertion of Sensitive Information into Externally-Accessible File or Directory"},
	{ID: "CWE-548", Name: "Exposure of Information Through Directory Listing"},
	{ID: "CWE-601", Name: "URL Redirection to Untrusted Site (Open Redirect)"},
	{ID: "CWE-611", Name: "Improper Restriction of XML External Entity Reference"},
	{ID: "CWE-613", Name: "Insufficient Session Expiration"},
	{ID: "CWE-639", Name: "Authorization Bypass Through User-Controlled Key (IDOR)"},
	{ID: "CWE-732", Name: "Incorrect Permission Assignment for Critical Resource"},
	{ID: "CWE-770", Name: "Allocation of Resources Without Limits or Throttling"},
	{ID: "CWE-798", Name: "Use of Hard-coded Credentials"},
	{ID: "CWE-862", Name: "Missing Authorization"},
	{ID: "CWE-863", Name: "Incorrect Authorization"},
	{ID: "CWE-915", Name: "Improperly Controlled Modification of Dynamically-Determined Object Attributes (Mass Assignment)"},
	{ID: "CWE-917", Name: "Improper Neutralization of Special Elements used in an Expression Language Statement"},
	{ID: "CWE-918", Name: "Server-Side Request Forgery (SSRF)"},
	{ID: "CWE-942", Name: "Permissive Cross-domain Policy with Untrusted Domains"},
	{ID: "CWE-1021", Name: "Improper Restriction of Rendered UI Layers or Frames (Clickjacking)"},
	{ID: "CWE-1336", Name: "Improper Neutralization of Special Elements Used in a Template Engine"},
}

var OWASP = []Entry{
	{ID: "A01:2021", Name: "Broken Access Control"},
	{ID: "A02:2021", Name: "Cryptographic Failures"},
	{ID: "A03:2021", Name: "Injection"},
	{ID: "A04:2021", Name: "Insecure Design"},
	{ID: "A05:2021", Name: "Security Misconfiguration"},
	{ID: "A06:2021", Name: "Vulnerable and Outdated Components"},
	{ID: "A07:2021", Name: "Identification and Authentication Failures"},
	{ID: "A08:2021", Name: "Software and Data Integrity Failures"},
	{ID: "A09:2021", Name: "Security Logging and Monitoring Failures"},
	{ID: "A10:2021", Name: "Server-Side Request Forgery"},
}

var Attack = []Entry{
	{ID: "T1595", Name: "Active Scanning", Tactic: "Reconnaissance"},
	{ID: "T1595.002", Name: "Active Scanning: Vulnerability Scanning", Tactic: "Reconnaissance"},
	{ID: "T1589", Name: "Gather Victim Identity Information", Tactic: "Reconnaissance"},
	{ID: "T1590", Name: "Gather Victim Network Information", Tactic: "Reconnaissance"},
	{ID: "T1592", Name: "Gather Victim Host Information", Tactic: "Reconnaissance"},
	{ID: "T1190", Name: "Exploit Public-Facing Application", Tactic: "Initial Access"},
	{ID: "T1133", Name: "External Remote Services", Tactic: "Initial Access"},
	{ID: "T1078", Name: "Valid Accounts", Tactic: "Initial Access"},
	{ID: "T1195", Name: "Supply Chain Compromise", Tactic: "Initial Access"},
	{ID: "T1199", Name: "Trusted Relationship", Tactic: "Initial Access"},
	{ID: "T1566", Name: "Phishing", Tactic: "Initial Access"},
	{ID: "T1059", Name: "Command and Scripting Interpreter", Tactic: "Execution"},
	{ID: "T1203", Name: "Exploitation for Client Execution", Tactic: "Execution"},
	{ID: "T1505.003", Name: "Server Software Component: Web Shell", Tactic: "Persistence"},
	{ID: "T1068", Name: "Exploitation for Privilege Escalation", Tactic: "Privilege Escalation"},
	{ID: "T1548", Name: "Abuse Elevation Control Mechanism", Tactic: "Privilege Escalation"},
	{ID: "T1110", Name: "Brute Force", Tactic: "Credential Access"},
	{ID: "T1110.003", Name: "Brute Force: Password Spraying", Tactic: "Credential Access"},
	{ID: "T1552", Name: "Unsecured Credentials", Tactic: "Credential Access"},
	{ID: "T1552.001", Name: "Unsecured Credentials: Credentials In Files", Tactic: "Credential Access"},
	{ID: "T1552.005", Name: "Unsecured Credentials: Cloud Instance Metadata API", Tactic: "Credential Access"},
	{ID: "T1539", Name: "Steal Web Session Cookie", Tactic: "Credential Access"},
	{ID: "T1550", Name: "Use Alternate Authentication Material", Tactic: "Defense Evasion"},
	{ID: "T1556", Name: "Modify Authentication Process", Tactic: "Credential Access"},
	{ID: "T1557", Name: "Adversary-in-the-Middle", Tactic: "Credential Access"},
	{ID: "T1606", Name: "Forge Web Credentials", Tactic: "Credential Access"},
	{ID: "T1040", Name: "Network Sniffing", Tactic: "Credential Access"},
	{ID: "T1046", Name: "Network Service Discovery", Tactic: "Discovery"},
	{ID: "T1083", Name: "File and Directory Discovery", Tactic: "Discovery"},
	{ID: "T1082", Name: "System Information Discovery", Tactic: "Discovery"},
	{ID: "T1087", Name: "Account Discovery", Tactic: "Discovery"},
	{ID: "T1518", Name: "Software Discovery", Tactic: "Discovery"},
	{ID: "T1526", Name: "Cloud Service Discovery", Tactic: "Discovery"},
	{ID: "T1210", Name: "Exploitation of Remote Services", Tactic: "Lateral Movement"},
	{ID: "T1021", Name: "Remote Services", Tactic: "Lateral Movement"},
	{ID: "T1213", Name: "Data from Information Repositories", Tactic: "Collection"},
	{ID: "T1005", Name: "Data from Local System", Tactic: "Collection"},
	{ID: "T1530", Name: "Data from Cloud Storage", Tactic: "Collection"},
	{ID: "T1041", Name: "Exfiltration Over C2 Channel", Tactic: "Exfiltration"},
	{ID: "T1567", Name: "Exfiltration Over Web Service", Tactic: "Exfiltration"},
	{ID: "T1498", Name: "Network Denial of Service", Tactic: "Impact"},
	{ID: "T1499", Name: "Endpoint Denial of Service", Tactic: "Impact"},
	{ID: "T1565", Name: "Data Manipulation", Tactic: "Impact"},
}

var (
	cweRe    = regexp.MustCompile(`^CWE-\d{1,5}$`)
	owaspRe  = regexp.MustCompile(`^A(0[1-9]|10):20\d\d$`)
	attackRe = regexp.MustCompile(`^T\d{4}(\.\d{3})?$`)
)

// Normalize canonicalises one identifier of a kind ("cwe", "owasp", "attack"): trims, upper-cases, accepts "79" or
// "cwe79" for CWE. It returns the id and whether it is well-formed. Unknown but well-formed ids are valid.
func Normalize(kind, id string) (string, bool) {
	s := strings.ToUpper(strings.TrimSpace(id))
	switch kind {
	case "cwe":
		s = strings.TrimPrefix(strings.TrimPrefix(s, "CWE"), "-")
		s = "CWE-" + strings.TrimLeft(s, " ")
		return s, cweRe.MatchString(s)
	case "owasp":
		return s, owaspRe.MatchString(s)
	case "attack":
		return s, attackRe.MatchString(s)
	}
	return s, false
}

// NormalizeList normalises, validates and de-duplicates a list; the first invalid id is returned as the error text.
func NormalizeList(kind string, ids []string) ([]string, string) {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		n, ok := Normalize(kind, id)
		if !ok {
			return nil, id
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out, ""
}

// Lookup resolves an id to its list entry (zero Entry when it is not in the curated list).
func Lookup(kind, id string) Entry {
	var list []Entry
	switch kind {
	case "cwe":
		list = CWE
	case "owasp":
		list = OWASP
	case "attack":
		list = Attack
	}
	for _, e := range list {
		if e.ID == id {
			return e
		}
	}
	return Entry{ID: id}
}

// Suggestion is a proposed tagging for a finding.
type Suggestion struct {
	CWE    []string `json:"cwe"`
	OWASP  []string `json:"owasp"`
	Attack []string `json:"attack"`
	Reason string   `json:"reason"`
}

type rule struct {
	pattern *regexp.Regexp
	cwe     []string
	owasp   []string
	attack  []string
	label   string
}

var rules = []rule{
	{regexp.MustCompile(`sql\s*inj|sqli|\bsql\b.*inject`), []string{"CWE-89"}, []string{"A03:2021"}, []string{"T1190"}, "SQL injection"},
	{regexp.MustCompile(`xss|cross[- ]site script`), []string{"CWE-79"}, []string{"A03:2021"}, []string{"T1190"}, "cross-site scripting"},
	{regexp.MustCompile(`ssrf|server[- ]side request`), []string{"CWE-918"}, []string{"A10:2021"}, []string{"T1190", "T1552.005"}, "server-side request forgery"},
	{regexp.MustCompile(`idor|insecure direct object|broken object level|bola`), []string{"CWE-639"}, []string{"A01:2021"}, []string{"T1190"}, "insecure direct object reference"},
	{regexp.MustCompile(`path traversal|directory traversal|\blfi\b|local file inclusion|arbitrary file read`), []string{"CWE-22"}, []string{"A01:2021"}, []string{"T1190", "T1083"}, "path traversal"},
	{regexp.MustCompile(`command injection|os command|\brce\b|remote code exec|code execution`), []string{"CWE-78", "CWE-94"}, []string{"A03:2021"}, []string{"T1190", "T1059"}, "remote code execution"},
	{regexp.MustCompile(`ssti|template injection`), []string{"CWE-1336"}, []string{"A03:2021"}, []string{"T1190"}, "template injection"},
	{regexp.MustCompile(`xxe|xml external`), []string{"CWE-611"}, []string{"A05:2021"}, []string{"T1190"}, "XML external entity"},
	{regexp.MustCompile(`deserializ`), []string{"CWE-502"}, []string{"A08:2021"}, []string{"T1190"}, "insecure deserialization"},
	{regexp.MustCompile(`csrf|cross[- ]site request forgery`), []string{"CWE-352"}, []string{"A01:2021"}, nil, "cross-site request forgery"},
	{regexp.MustCompile(`open redirect`), []string{"CWE-601"}, []string{"A01:2021"}, []string{"T1566"}, "open redirect"},
	{regexp.MustCompile(`file upload|unrestricted upload|web ?shell`), []string{"CWE-434"}, []string{"A04:2021"}, []string{"T1190", "T1505.003"}, "unrestricted file upload"},
	{regexp.MustCompile(`default cred|weak password|hard-?coded|default password|weak credential`), []string{"CWE-798"}, []string{"A07:2021"}, []string{"T1078"}, "default or hard-coded credentials"},
	{regexp.MustCompile(`brute|rate limit|no lockout|credential stuffing|password spray`), []string{"CWE-307"}, []string{"A07:2021"}, []string{"T1110"}, "missing brute-force protection"},
	{regexp.MustCompile(`auth(entication)? bypass|missing auth|unauthenticated|no authentication`), []string{"CWE-306", "CWE-287"}, []string{"A07:2021"}, []string{"T1190", "T1078"}, "missing or bypassable authentication"},
	{regexp.MustCompile(`broken access|privilege escalation|missing authorization|forced browsing|access control`), []string{"CWE-862", "CWE-284"}, []string{"A01:2021"}, []string{"T1068", "T1548"}, "broken access control"},
	{regexp.MustCompile(`session (fixation|expir|management)|cookie flags|session id`), []string{"CWE-384", "CWE-613"}, []string{"A07:2021"}, []string{"T1539"}, "session management"},
	{regexp.MustCompile(`jwt|json web token`), []string{"CWE-347"}, []string{"A02:2021"}, []string{"T1550"}, "weak JWT handling"},
	{regexp.MustCompile(`cors`), []string{"CWE-942", "CWE-346"}, []string{"A05:2021"}, nil, "permissive CORS"},
	{regexp.MustCompile(`clickjack|x-frame|frame-ancestors`), []string{"CWE-1021"}, []string{"A05:2021"}, nil, "clickjacking"},
	{regexp.MustCompile(`directory listing|index of /`), []string{"CWE-548"}, []string{"A05:2021"}, []string{"T1083"}, "directory listing"},
	{regexp.MustCompile(`\.git\b|exposed (file|backup|config)|\.env\b|backup file|credentials in file|secret.*(repo|file)`), []string{"CWE-200", "CWE-538"}, []string{"A05:2021"}, []string{"T1552.001"}, "exposed sensitive file"},
	{regexp.MustCompile(`information (disclosure|leak|exposure)|info leak|verbose error|stack trace|banner`), []string{"CWE-200", "CWE-209"}, []string{"A05:2021"}, []string{"T1592"}, "information disclosure"},
	{regexp.MustCompile(`misconfig|security header|hsts|insecure configuration`), []string{"CWE-16"}, []string{"A05:2021"}, nil, "security misconfiguration"},
	{regexp.MustCompile(`tls|ssl|cleartext|plaintext|http only|weak cipher|certificate`), []string{"CWE-319", "CWE-326"}, []string{"A02:2021"}, []string{"T1040"}, "weak transport security"},
	{regexp.MustCompile(`outdated|end[- ]of[- ]life|known vulnerabilit|cve-\d{4}|vulnerable component|unpatched`), nil, []string{"A06:2021"}, []string{"T1190", "T1595.002"}, "vulnerable or outdated component"},
	{regexp.MustCompile(`denial of service|\bdos\b|resource exhaustion|redos`), []string{"CWE-400", "CWE-770"}, nil, []string{"T1499"}, "denial of service"},
	{regexp.MustCompile(`mass assignment`), []string{"CWE-915"}, []string{"A04:2021"}, []string{"T1190"}, "mass assignment"},
	{regexp.MustCompile(`request smuggling|desync`), []string{"CWE-444"}, []string{"A05:2021"}, []string{"T1190"}, "HTTP request smuggling"},
	{regexp.MustCompile(`log(ging)? (injection|missing)|no audit|monitoring`), []string{"CWE-532"}, []string{"A09:2021"}, nil, "logging and monitoring"},
}

// Suggest proposes tags from a finding's vulnerability class and name. It merges every matching rule, so a finding
// named "SQL injection leading to RCE" gets both. An empty result means nothing matched.
func Suggest(texts ...string) Suggestion {
	hay := strings.ToLower(strings.Join(texts, " | "))
	var sg Suggestion
	var labels []string
	seen := map[string]bool{}
	add := func(dst *[]string, ids []string) {
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				*dst = append(*dst, id)
			}
		}
	}
	for _, r := range rules {
		if r.pattern.MatchString(hay) {
			add(&sg.CWE, r.cwe)
			add(&sg.OWASP, r.owasp)
			add(&sg.Attack, r.attack)
			labels = append(labels, r.label)
		}
	}
	sort.Strings(sg.CWE)
	sort.Strings(sg.OWASP)
	sort.Strings(sg.Attack)
	if len(labels) > 0 {
		sg.Reason = "matched: " + strings.Join(labels, ", ")
	}
	if sg.CWE == nil {
		sg.CWE = []string{}
	}
	if sg.OWASP == nil {
		sg.OWASP = []string{}
	}
	if sg.Attack == nil {
		sg.Attack = []string{}
	}
	return sg
}
