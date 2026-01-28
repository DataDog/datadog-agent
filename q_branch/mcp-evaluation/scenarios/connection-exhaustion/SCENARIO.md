# Connection Exhaustion

## Difficulty
Medium

## Problem Statement
A service is opening TCP connections without properly closing them, leading to connection exhaustion and file descriptor leaks.

## Symptoms
Observable symptoms that an SRE would notice:
- High number of connections in ESTABLISHED or CLOSE_WAIT state
- Connections from single process accumulating over time
- File descriptor count for process increasing
- Eventually hits ulimit or system connection limits
- "Too many open files" errors may appear

## Root Cause
Application opening sockets/connections but not explicitly closing them. Connections remain open even after use, consuming file descriptors and connection table entries.

## Investigation Steps
Recommended approach to diagnose:
1. **Check connections**: Use `get_network_connections` to see high connection count
2. **Find source process**: Identify PID with most connections
3. **Check file descriptors**: Use `get_process_info` to see FD count growing
4. **Monitor growth**: Track connection count over time
5. **Check connection states**: Look for accumulation in specific states

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified process with excessive connections (PID and count)
- Resource issue: Connection leak/exhaustion
- Root cause: Connections not being closed properly

### Alternative Valid Diagnoses (Partial Credit)
- "Connection leak"
- "Too many open connections from one process"
- "File descriptor leak from network connections"

### Key Terms (should be mentioned)
- "connection", "leak", "not closed"
- "file descriptor" or "socket"
- Connection count or FD count trend

### Common Errors (deductions)
- Confusing with network bandwidth issue (-20%)
- Not identifying source process (-30%)
- Blaming remote endpoint (-15%)

## Setup Instructions
How to deploy the scenario:
```bash
cd /mcp/scenarios/connection-exhaustion
python3 workload.py > /tmp/connection-exhaustion.log 2>&1 &
echo $! > /tmp/connection-exhaustion.pid
```

## Cleanup Instructions
How to stop and clean up:
```bash
if [ -f /tmp/connection-exhaustion.pid ]; then
    kill $(cat /tmp/connection-exhaustion.pid)
    rm /tmp/connection-exhaustion.pid
fi
rm -f /tmp/connection-exhaustion.log
```

## Expected Timeline
How quickly the issue manifests:
- **Short term** (2-5 minutes)
- Opens 10 connections/second = 600/minute
- Should have 500+ connections within 2-3 minutes

## Success Criteria
What indicates a correct diagnosis:
- [x] Identified process with connection leak
- [x] Documented connection count growth
- [x] Understood root cause: Connections not closed
- [x] Proposed mitigation: Fix code to close connections or restart service

## Autograder Scoring Rubric

### Connection Growth Identification (25 points)
- 25pts: Showed connections growing with measurements
- 15pts: Identified high connection count without trend
- 0pts: Did not identify connection issue

### Process Identification (25 points)
- 25pts: Found process (PID and name) with connection leak
- 10pts: Mentioned process without specifics
- 0pts: Did not identify process

### Root Cause Analysis (30 points)
- 30pts: Explained connections not being closed/released
- 20pts: Mentioned connection leak without explanation
- 10pts: Only reported symptoms
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (fix code, restart, add connection pooling)
- 10pts: Generic solution
- 0pts: No mitigation
