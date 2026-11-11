---
name: hunt-endpoint
description: Endpoint threat-hunting skill. Invoke when asked to hunt for compromise indicators, persistence mechanisms, or attacker tools on individual hosts during an authorized engagement.
---

# Endpoint Threat Hunt

Systematic search for adversary presence on individual hosts under an **authorized** engagement. Focus on persistence, privilege escalation residue, credential harvesting artifacts, and living-off-the-land binaries (LOLBins).

---

## Scope and constraints

| Allowed | Prohibited |
|---|---|
| Read-only artifact collection (logs, registry, process list, file hashes) | Modifying registry keys, files, or process state |
| Running read-only scripts (PowerShell `-NonInteractive -NoProfile`, bash read-only) | Installing tools without change-management approval |
| Memory acquisition on authorized hosts | Running unsigned code that bypasses EDR on production hosts |
| Offline analysis of acquired images/logs | Exfiltrating credentials or PII outside of authorized evidence collection |

---

## Hunt phases

### 1. Process and service inventory
```
# Linux
ps auxf; systemctl list-units --type=service --state=running
# Windows (PowerShell)
Get-Process | Select Name,Id,Path,CPU | Sort CPU -Desc | Select -First 50
Get-Service | Where Status -eq Running | Select Name,DisplayName,StartType
```
- Flag: processes with no on-disk binary (`/proc/<pid>/exe` → deleted), unusual parent-child chains (`cmd.exe` spawned by `winword.exe`), unsigned PE images in temp dirs.

### 2. Persistence mechanisms
```
# Windows autorun locations
reg query HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run
reg query HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run
schtasks /query /fo LIST /v | findstr "Task Name\|Run As User\|Task To Run"
# Linux cron / systemd / rc.local / init.d
crontab -l; cat /etc/cron*/* /etc/rc.local 2>/dev/null; ls /etc/systemd/system/
```
- Flag: entries pointing to temp dirs, encoded commands, LOLBIN chains, recently modified scheduled tasks.

### 3. User and credential artifacts
- Enumerate local accounts and recent logins: `last`, `lastlog`, `Get-LocalUser`, `net user`.
- Check for credential-dumping residue: SAM/SYSTEM shadow copies, lsass memory dumps, mimikatz artifacts in temp.
- Review bash/PowerShell history for sensitive commands.

### 4. File system anomalies
- Recently modified executables: `find / -type f -newer /tmp/baseline_time -perm /111 2>/dev/null`.
- Unusual hidden files: `find / -name ".*" -not -path "/proc/*" -not -path "/sys/*" 2>/dev/null | head -50`.
- Packed/obfuscated binaries: `file <binary>` + `strings <binary> | wc -l` (low string count = likely packed).
- Large archives in temp dirs (data staging).

### 5. Network connections from host
```
ss -tulnp   # Linux
netstat -ano  # Windows
```
- Flag: ESTABLISHED connections on uncommon ports, listening services on 0.0.0.0 that weren't there before.

### 6. Log analysis
- Auth logs: `/var/log/auth.log`, Windows Event IDs 4624/4625/4648/4672.
- Lateral movement indicators: 4648 (explicit creds), 4776 (NTLM auth), 7045 (new service).
- PowerShell logging: Event ID 4103/4104 (script block), look for encoded/obfuscated scripts.

### 7. Memory analysis (if image available)
Use Volatility 3:
```
vol -f <image> windows.pslist
vol -f <image> windows.netstat
vol -f <image> windows.malfind   # injected/hollow processes
vol -f <image> windows.cmdline
```

---

## Key tools
`ps`, `ss`/`netstat`, `find`, `file`, `strings`, `lsof`, `auditd`/`ausearch`, `osquery`, `velociraptor`, `Volatility 3`, `autoruns` (Sysinternals), `Sysmon`.

## Output format
```
## Endpoint Hunt Report — <HOST> — <DATE>
### System baseline (OS, uptime, users)
### Findings (severity: Critical/High/Medium/Low/Info)
  - [T-ID] <Technique>: <Evidence path / command output excerpt> — <Verdict>
### Persistence artifacts discovered
### Credential exposure indicators
### Recommended remediation
```
