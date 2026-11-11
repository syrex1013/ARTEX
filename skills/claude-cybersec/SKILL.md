---
name: claude-cybersec
description: General cybersecurity assistant skill. Invoke for security analysis, CVE research, threat modeling, security architecture review, and general security questions in the context of an authorized engagement.
---

# Claude Cybersecurity Assistant

General-purpose security reasoning skill for authorized engagements and research. Use for analysis, planning, research, and guidance when no more specific skill applies.

---

## Capabilities

### CVE research and analysis
- Analyze CVE descriptions and assess exploitability in a given target environment.
- Correlate CVE with CVSS score, EPSS probability, KEV status.
- Find public PoC availability: ExploitDB, GitHub, Metasploit module search.
- Determine patch availability and version constraints.

### Threat modeling
Supported frameworks: STRIDE, PASTA, LINDDUN, TARA (automotive), MITRE ATT&CK.
- Identify assets, trust boundaries, data flows for a described system.
- Enumerate threats per component.
- Rate likelihood × impact for each threat.
- Recommend controls mapped to NIST CSF / ISO 27001 / CIS Controls.

### Security architecture review
- Review network diagrams or descriptions for segmentation weaknesses.
- Assess authentication/authorization design (OAuth flows, JWT usage, API key management).
- Identity and secrets management review.
- Logging and monitoring gaps.
- Zero-trust readiness checklist.

### Incident response guidance
- Triage: scope the incident, identify affected systems, preserve evidence.
- Containment options: isolation, credential rotation, firewall rule.
- Eradication: persistence removal, patch, harden.
- Recovery: restore from clean backup, re-validate.
- Lessons learned: timeline reconstruction, root cause, control gaps.

### Malware analysis (static/behavioral)
- File metadata: `file`, `strings`, `exiftool`, `pefile` (Python).
- Hashing and VirusTotal correlation.
- Import table analysis: suspicious API calls (VirtualAlloc, CreateRemoteThread, WriteProcessMemory).
- Network IOC extraction: `strings | grep -E '\b([0-9]{1,3}\.){3}[0-9]{1,3}\b|[a-z0-9\-]+\.[a-z]{2,}'`.
- YARA rule writing for detected patterns.

### Security code review
- Identify injection sinks (SQL, OS command, LDAP, XPath).
- Unsafe deserialization patterns.
- Cryptographic weaknesses: hardcoded keys, ECB mode, MD5/SHA1 for passwords.
- Race conditions in auth/session flows.
- Supply chain: dependency confusion, typosquatting risk in package.json/requirements.txt.

### Report writing assistance
- Executive summary from technical findings.
- Risk rating justification (CVSS, OWASP, business impact).
- Remediation priority matrix.
- Compliance gap mapping (PCI-DSS, SOC2, HIPAA, ISO27001).

---

## How to use this skill

Describe your analysis need in natural language. Examples:
- "Analyze this nmap output and suggest follow-on tests."
- "I found command injection in a PHP app — help me build the reproduction steps and write the finding."
- "Threat-model this microservices architecture: [describe it]."
- "This binary imports CreateRemoteThread and VirtualAllocEx — what should I look for next?"
- "Draft the executive summary for this pentest: [paste findings list]."

---

## Ethics and authorization reminder

This skill operates only within **authorized** engagements. All techniques described are for defensive understanding and authorized offensive security work. Never use this guidance to attack systems without explicit written authorization from the system owner.
