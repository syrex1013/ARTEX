---
name: password-audit
description: Password security audit skill. Invoke when performing password hash cracking, policy assessment, or credential hygiene review during an authorized engagement.
---

# Password Security Audit

Assess password security posture and crack captured hashes under an **authorized** engagement. Document findings for remediation.

---

## Scope constraints

| Allowed | Prohibited |
|---|---|
| Cracking hashes obtained from in-scope systems | Obtaining hashes via unauthorized access |
| Testing password policy with provided test accounts | Brute-forcing live authentication without written approval and rate-limit awareness |
| Offline hash cracking | Cracking hashes beyond what's needed to prove weakness |
| Assessing policy configuration | Sharing cracked credentials outside the engagement team |

---

## Hash identification
```bash
hashid <HASH>           # identify hash type
hash-identifier <HASH>  # alternative
# Common types: NTLM (32 hex), bcrypt ($2y$), SHA-256 (64 hex), MD5 (32 hex)
```

---

## Hashcat

### Common attack modes
```bash
# Wordlist (most effective first)
hashcat -m <TYPE> -a 0 hashes.txt /usr/share/wordlists/rockyou.txt

# Wordlist + rules (best coverage)
hashcat -m <TYPE> -a 0 hashes.txt /usr/share/wordlists/rockyou.txt -r /usr/share/hashcat/rules/best64.rule
hashcat -m <TYPE> -a 0 hashes.txt /usr/share/wordlists/rockyou.txt -r /usr/share/hashcat/rules/OneRuleToRuleThemAll.rule

# Mask attack (pattern-based: Company2024!)
hashcat -m <TYPE> -a 3 hashes.txt "?u?l?l?l?l?l?l?d?d?d?s"

# Combination attack
hashcat -m <TYPE> -a 1 hashes.txt words1.txt words2.txt

# NTLM
hashcat -m 1000 ntlm_hashes.txt rockyou.txt -r rules/best64.rule

# NTLMv2 (Net-NTLMv2 from Responder)
hashcat -m 5600 netntlmv2.txt rockyou.txt

# Kerberos AS-REP (AS-REP roast)
hashcat -m 18200 asrep_hashes.txt rockyou.txt

# Kerberos TGS (Kerberoast)
hashcat -m 13100 tgs_hashes.txt rockyou.txt

# bcrypt (slow — use targeted wordlist)
hashcat -m 3200 bcrypt_hashes.txt curated_passwords.txt
```

### Custom wordlist generation
```bash
# Company-specific mutations
cewl https://<TARGET_WEBSITE> -d 3 -w cewl_words.txt
# Season + year patterns
python3 -c "
years=['2022','2023','2024','2025']; seasons=['Spring','Summer','Autumn','Winter','Fall']
for s in seasons:
  for y in years:
    print(f'{s}{y}'); print(f'{s}{y}!'); print(f'{s}@{y}')
" > seasonal.txt
# Combine with company name
hashcat -m 1000 ntlm.txt cewl_words.txt seasonal.txt -a 1
```

---

## John the Ripper

```bash
# Auto-detect format
john hashes.txt --wordlist=/usr/share/wordlists/rockyou.txt

# Specific format
john --format=NT hashes.txt --wordlist=rockyou.txt --rules=All

# Show cracked
john hashes.txt --show
```

---

## Password policy assessment

### Windows (domain)
```powershell
Get-ADDefaultDomainPasswordPolicy
Get-ADFineGrainedPasswordPolicy -Filter *
# Check: MinPasswordLength, ComplexityEnabled, MaxPasswordAge, LockoutThreshold
```

### Linux PAM
```bash
cat /etc/pam.d/common-password
# Check: minlen, dcredit, ucredit, lcredit, ocredit, remember
cat /etc/login.defs | grep -i pass
```

### Key metrics to report
- Minimum password length (< 12 = finding)
- Complexity requirements (none = finding)
- Lockout threshold (none or > 10 = finding)
- Password history (< 24 = finding for high-security)
- Maximum age (> 180 days or no expiry = finding)
- Multi-factor authentication coverage

---

## Credential stuffing simulation (test accounts only)
```bash
# Test with known breached passwords for test users
# Never run against production auth endpoints at scale
hydra -L test_users.txt -P test_passwords.txt <TARGET> http-post-form "/login:user=^USER^&pass=^PASS^:Invalid" -t 1 -w 3
```

---

## Statistics and reporting
```bash
# After cracking, analyze patterns
pipal cracked_passwords.txt
# Gives: top passwords, lengths, character sets, patterns
```

---

## Output format
```
## Password Audit Report — <ENGAGEMENT> — <DATE>
### Hash types discovered
### Cracking results
  - Total hashes: N
  - Cracked: N (XX%)
  - Time to crack (median / 50th percentile): <time>
### Top cracked passwords (anonymized if needed)
### Password pattern analysis
### Policy assessment
  | Control | Current | Recommended | Finding? |
### Findings
  #### [HIGH/MEDIUM/LOW] <Finding>
  - Evidence:
  - Remediation:
### Cracked credential list (handle as sensitive — store encrypted)
```
