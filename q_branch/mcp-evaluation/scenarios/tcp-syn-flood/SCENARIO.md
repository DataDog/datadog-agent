# TCP SYN Flood

## Difficulty
Hard

## Problem Statement
SYN flood attack pattern where many half-open TCP connections accumulate, filling the SYN queue and preventing legitimate connections from being established.

## Symptoms
Observable symptoms that an SRE would notice:
- Many connections in SYN_RECV state
- New connection attempts timing out or being rejected
- SYN queue overflow messages in kernel logs
- Legitimate services unable to accept connections
- `ss` or `netstat` shows high count of SYN_RECV

## Root Cause
Attacker (or buggy client) sending many SYN packets without completing TCP handshake (missing ACK). Server allocates resources for half-open connections, filling SYN queue and preventing new connections.

## Investigation Steps
Recommended approach to diagnose:
1. **Check connections**: Use `get_network_connections` to see SYN_RECV states
2. **Count SYN_RECV**: Use `bash_execute` with `ss -tan | grep SYN-RECV | wc -l`
3. **Check kernel logs**: Use `read_file` on /var/log/kern.log for SYN flood messages
4. **Check SYN queue**: Use `bash_execute` with `ss -lnt` to see queue depths
5. **Identify target**: Find service being flooded

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified many SYN_RECV connections
- Found SYN queue exhaustion preventing new connections
- Root cause: SYN flood attack or incomplete handshakes

### Alternative Valid Diagnoses (Partial Credit)
- "SYN flood attack"
- "Half-open connections filling queue"
- "Connection refused due to SYN queue"

### Key Terms (should be mentioned)
- "SYN", "SYN_RECV", "half-open"
- "SYN flood" or "SYN queue"
- Three-way handshake

### Common Errors (deductions)
- Confusing with legitimate high traffic (-20%)
- Not understanding TCP handshake (-25%)
- Blaming application instead of network layer (-20%)

## Setup Instructions
```bash
cd /mcp/scenarios/tcp-syn-flood
python3 workload.py > /tmp/tcp-syn-flood.log 2>&1 &
echo $! > /tmp/tcp-syn-flood.pid
```

## Cleanup Instructions
```bash
if [ -f /tmp/tcp-syn-flood.pid ]; then
    kill $(cat /tmp/tcp-syn-flood.pid)
    rm /tmp/tcp-syn-flood.pid
fi
rm -f /tmp/tcp-syn-flood.log
```

## Expected Timeline
- **Short term** (2-3 minutes)
- SYN_RECV connections accumulate quickly
- SYN queue fills in 2-3 minutes

## Success Criteria
- [x] Identified SYN_RECV accumulation
- [x] Found SYN queue exhaustion
- [x] Understood TCP handshake attack
- [x] Proposed mitigation: SYN cookies, firewall rules, rate limiting

## Autograder Scoring Rubric

### SYN_RECV Identification (25 points)
- 25pts: Found many SYN_RECV with count
- 15pts: Mentioned connection issue without state
- 0pts: Did not identify SYN_RECV

### Queue Exhaustion (25 points)
- 25pts: Identified SYN queue full/overflow
- 10pts: Mentioned connection problems
- 0pts: Did not identify queue issue

### Root Cause Analysis (30 points)
- 30pts: Explained SYN flood or incomplete handshakes
- 20pts: Mentioned SYN attack without explanation
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (SYN cookies, firewall, rate limit)
- 10pts: Generic solution
- 0pts: No mitigation
