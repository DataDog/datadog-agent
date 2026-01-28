# DNS Resolution Failure

## Difficulty
Easy

## Problem Statement
DNS resolution is failing due to misconfigured nameserver settings, preventing services from resolving hostnames while direct IP connections still work.

## Symptoms
Observable symptoms that an SRE would notice:
- "Name or service not known" errors in logs
- "Could not resolve hostname" or similar errors
- Connections to IPs work but hostnames fail
- /etc/resolv.conf points to invalid or unreachable nameserver
- DNS queries timing out

## Root Cause
Misconfigured /etc/resolv.conf with invalid nameserver IP address (e.g., 192.0.2.1 which is TEST-NET documentation address, not a real DNS server).

## Investigation Steps
Recommended approach to diagnose:
1. **Check service logs**: Use `tail_file` or `read_file` on logs to see DNS errors
2. **Test DNS resolution**: Use `check_connectivity` MCP tool with hostname
3. **Check DNS config**: Use `read_file` on /etc/resolv.conf
4. **Verify nameserver**: Check if nameserver IP is reachable
5. **Compare with working config**: Check if backup resolv.conf exists

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified DNS resolution failures in logs
- Found misconfigured /etc/resolv.conf with invalid nameserver
- Root cause: Invalid DNS server configuration

### Alternative Valid Diagnoses (Partial Credit)
- "DNS not working"
- "Invalid nameserver in resolv.conf"
- "Cannot resolve hostnames"

### Key Terms (should be mentioned)
- "DNS", "resolv.conf", "nameserver"
- "resolution failure", "cannot resolve"
- Invalid nameserver IP address

### Common Errors (deductions)
- Blaming network connectivity (-20%): IPs work, only DNS fails
- Not checking /etc/resolv.conf (-35%)
- Confusing with routing issue (-20%)

## Setup Instructions
How to deploy the scenario:
```bash
cd /mcp/scenarios/dns-resolution-failure

# Backup original resolv.conf
sudo cp /etc/resolv.conf /etc/resolv.conf.backup

# Replace with broken config
sudo cp resolv.conf.broken /etc/resolv.conf

# Start service
python3 workload.py > /tmp/dns-failure.log 2>&1 &
echo $! > /tmp/dns-failure.pid
```

## Cleanup Instructions
How to stop and clean up:
```bash
# Stop process
if [ -f /tmp/dns-failure.pid ]; then
    kill $(cat /tmp/dns-failure.pid)
    rm /tmp/dns-failure.pid
fi
rm -f /tmp/dns-failure.log

# Restore original resolv.conf
if [ -f /etc/resolv.conf.backup ]; then
    sudo mv /etc/resolv.conf.backup /etc/resolv.conf
fi
```

## Expected Timeline
How quickly the issue manifests:
- **Immediate** (< 1 minute)
- DNS failures appear in logs within seconds
- Every connection attempt fails immediately

## Success Criteria
What indicates a correct diagnosis:
- [x] Identified DNS resolution failures in logs
- [x] Found misconfigured /etc/resolv.conf
- [x] Identified invalid nameserver IP
- [x] Proposed mitigation: Fix /etc/resolv.conf with valid nameserver

## Autograder Scoring Rubric

### Symptom Identification (25 points)
- 25pts: Found DNS resolution errors in logs with examples
- 15pts: Mentioned DNS errors without specifics
- 0pts: Did not identify DNS failures

### Configuration Issue (25 points)
- 25pts: Found misconfigured /etc/resolv.conf with invalid nameserver
- 15pts: Checked resolv.conf but didn't identify invalid nameserver
- 0pts: Did not check DNS configuration

### Root Cause Analysis (30 points)
- 30pts: Explained invalid nameserver causing DNS failures
- 20pts: Mentioned DNS misconfiguration without details
- 10pts: Only reported symptoms
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (fix resolv.conf with valid nameserver)
- 10pts: Generic solution
- 0pts: No mitigation
