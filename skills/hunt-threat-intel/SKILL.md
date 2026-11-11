---
name: hunt-threat-intel
description: Threat intelligence enrichment skill. Invoke when you need to enrich IPs, domains, file hashes, or CVEs with threat-intelligence context during an authorized engagement.
---

# Threat Intelligence Enrichment

Enrich indicators of compromise (IoCs) with open-source threat intelligence (OSINT) during an **authorized** engagement. Cross-reference against known threat actors, malware families, and vulnerability databases.

---

## Indicator types and enrichment sources

### IP addresses
1. Reverse DNS: `dig -x <IP>` / `host <IP>`.
2. ASN / hosting: `whois <IP>` → flag: hosting providers known for bulletproof hosting.
3. OSINT reputation: check against AbuseIPDB, Shodan (if key available), GreyNoise.
4. Threat feeds: match against freely available blocklists (Emerging Threats, Feodo C2 tracker).
5. Geo/ISP context: flag IPs in unexpected jurisdictions given the engagement scope.

### Domains / hostnames
1. WHOIS registration age and registrar: recently registered domains (<30 days) are high-risk.
2. DNS history: passive DNS to identify domain shadowing or fast-flux.
3. Certificate transparency: `crt.sh` — look for wildcard certs, recently issued certs for look-alike domains.
4. Alexa/Tranco rank: unranked domain communicating with internal hosts is suspicious.
5. DGA check: high-entropy labels (entropy >3.5 bits/char), uncommon TLDs (.xyz, .top, .tk, .bit).

### File hashes (MD5/SHA1/SHA256)
1. VirusTotal public API (if key configured): detection ratio, first/last seen, behavior tags.
2. MalwareBazaar: `curl "https://mb-api.abuse.ch/api/v1/" -d "query=get_info&hash=<SHA256>"`.
3. NSRL: check if hash is in known-good set (reduces false positives).
4. PE metadata: imphash, PEiD packer signature, compile timestamp (if available).

### CVEs
1. NVD: `curl "https://services.nvd.nist.gov/rest/json/cves/2.0?cveId=<CVE-ID>"`.
2. EPSS score (exploitation likelihood): `curl "https://api.first.org/data/v1/epss?cve=<CVE-ID>"`.
3. ExploitDB / Metasploit module existence.
4. KEV: check against CISA Known Exploited Vulnerabilities catalog.

---

## ATT&CK actor correlation

When an IoC matches a known threat actor:
1. Map to MITRE ATT&CK group (G-ID).
2. List that group's common techniques — prioritize for follow-on hunts.
3. Document campaign name and victimology to assess targeting likelihood.

---

## Automation helpers
```bash
# Bulk IP reputation check (requires jq)
for ip in $(cat ips.txt); do
  curl -sG "https://api.abuseipdb.com/api/v2/check" \
    -H "Key: $ABUSEIPDB_KEY" -H "Accept: application/json" \
    --data-urlencode "ipAddress=$ip" | jq -r '.data | "\(.ipAddress) score=\(.abuseConfidenceScore) country=\(.countryCode)"'
done
# Domain entropy check (Python one-liner)
python3 -c "import math,sys; d=sys.argv[1]; s=-sum(d.count(c)/len(d)*math.log2(d.count(c)/len(d)) for c in set(d)); print(f'{d} entropy={s:.2f}')" example.com
```

---

## Output format
For each IoC, produce:
```
IoC: <value>
Type: IP | Domain | Hash | CVE
Verdict: Malicious | Suspicious | Benign | Unknown
Confidence: High | Medium | Low
Sources: [list of sources checked]
Threat actor / malware family: (if known)
ATT&CK technique: T-XXXX (if applicable)
Notes: <context>
```
