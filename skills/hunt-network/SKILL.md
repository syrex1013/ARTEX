---
name: hunt-network
description: Network threat-hunting skill. Invoke when asked to hunt for threats, anomalies or attacker activity across network infrastructure during an authorized engagement.
---

# Network Threat Hunt

Systematic adversary-focused search across network infrastructure under an **authorized** engagement. The goal is to uncover indicators of compromise, lateral movement, C2 beaconing, and data staging that routine monitoring missed.

---

## Scope and constraints

| Allowed | Prohibited |
|---|---|
| Passive capture and analysis of traffic on authorized segments | Capturing traffic on segments outside scope |
| Active scanning of in-scope hosts | Exploitation or modification of discovered services |
| Credential-free banner grabs, port scans, service fingerprinting | Brute-force or credential-stuffing |
| Log analysis and timeline reconstruction | Deleting or altering evidence |

---

## Hunt phases

### 1. Baseline collection
- Enumerate active hosts: `nmap -sn <CIDR>` → build asset inventory.
- Port/service scan in-scope hosts: `nmap -sV -p- --min-rate 2000 <TARGETS>`.
- Pull DHCP/DNS logs if accessible (look for fast-flux, long TTL abuse, uncommon TLDs).

### 2. Network traffic analysis
- Capture a rolling sample: `tcpdump -i <IF> -w /tmp/hunt-$(date +%s).pcap -G 300 -W 2`.
- Identify beaconing: regular small outbound intervals → `tshark -r <PCAP> -T fields -e ip.dst -e frame.time_relative | sort | uniq -c | sort -rn | head -40`.
- Look for DNS tunnelling: unusually long hostnames, high-entropy subdomains, TXT record requests at scale.
- Find C2 candidates: long-duration TCP sessions, connections to Tor exit nodes, JA3/JA3S TLS fingerprints matching known malware families.
- Identify lateral movement: SMB tree connects, WinRM/PSRemoting, unusual RDP chains, `pass-the-hash` NTLM patterns.

### 3. Protocol anomaly checks
- HTTP/S: unusual user-agent strings, encoded payloads in params, POST to uncommon paths.
- DNS: query volume spikes, NX-domain storms, reverse-lookup scans.
- SMTP: relaying anomalies, BCC exfil patterns, spoofed From headers.
- FTP/SFTP/SCP: unexpected file transfers, large archives, odd hours.

### 4. Host pivot indicators
- Identify assets with outbound connections on non-standard ports.
- Cross-reference with threat-intel IPs/domains (see `hunt-threat-intel` skill).
- Note any host communicating with >10 unique external IPs/hour (potential scanner or exfil agent).

### 5. Timeline and reporting
- Build event timeline: `first_seen`, `last_seen`, `src_ip`, `dst_ip`, `port`, `bytes`, `verdict`.
- Classify each finding: `True Positive`, `False Positive`, `Needs Review`.
- For each TP: document MITRE ATT&CK technique (T-ID), evidence, severity, recommended containment.

---

## Key tools
`nmap`, `masscan`, `tcpdump`, `tshark`/`wireshark`, `zeek`, `suricata`, `bettercap` (passive), `netstat`/`ss`, `arp-scan`, `p0f`, `zmap`.

## Output format
Return a structured hunt report:
```
## Network Hunt Report — <ENGAGEMENT> — <DATE>
### Executive summary
### Assets discovered
### Anomalies found (severity: Critical/High/Medium/Low/Info)
  - [T-ID] <Technique>: <Evidence> — <Verdict>
### Recommended containment actions
### Evidence artifacts (file paths / packet offsets)
```
