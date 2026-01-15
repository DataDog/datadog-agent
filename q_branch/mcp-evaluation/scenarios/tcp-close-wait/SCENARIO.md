# TCP CLOSE_WAIT

## Difficulty
Hard

## Problem Statement
HTTP server accumulating connections in CLOSE_WAIT state due to not properly closing sockets, leading to file descriptor exhaustion.

## Symptoms
Observable symptoms that an SRE would notice:
- Many connections in CLOSE_WAIT state (visible in netstat/ss)
- CLOSE_WAIT count increasing over time
- File descriptor count for server process growing
- Eventually hits FD limit causing accept() failures
- Client connections work initially but server doesn't clean up

## Root Cause
Server application not explicitly closing sockets after handling requests. When client closes connection, socket enters CLOSE_WAIT on server side waiting for server to close, but application never calls close().

## Investigation Steps
Recommended approach to diagnose:
1. **Check connections**: Use `get_network_connections` to see CLOSE_WAIT states
2. **Count CLOSE_WAIT**: Use `bash_execute` with `ss -tan | grep CLOSE-WAIT | wc -l`
3. **Find server process**: Identify process with many CLOSE_WAIT connections
4. **Check FD count**: Use `get_process_info` to see FD growth
5. **Monitor over time**: Confirm CLOSE_WAIT count increases

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified accumulation of CLOSE_WAIT connections
- Found server process not closing sockets
- Root cause: Missing socket close() after handling requests

### Alternative Valid Diagnoses (Partial Credit)
- "CLOSE_WAIT connections piling up"
- "Server not closing sockets properly"
- "Socket leak in server"

### Key Terms (should be mentioned)
- "CLOSE_WAIT", "socket", "not closed"
- Connection state
- Server process

### Common Errors (deductions)
- Confusing with TIME_WAIT (-20%)
- Blaming client (-25%)
- Not understanding CLOSE_WAIT state (-20%)

## Setup Instructions
```bash
cd /mcp/scenarios/tcp-close-wait
python3 server.py > /tmp/tcp-close-wait.log 2>&1 &
echo $! > /tmp/tcp-close-wait.pid

# Generate client requests
for i in {1..100}; do curl -s http://localhost:9000/ >/dev/null & done
```

## Cleanup Instructions
```bash
if [ -f /tmp/tcp-close-wait.pid ]; then
    kill $(cat /tmp/tcp-close-wait.pid)
    rm /tmp/tcp-close-wait.pid
fi
rm -f /tmp/tcp-close-wait.log
```

## Expected Timeline
- **Short term** (3-5 minutes)
- With client requests, CLOSE_WAIT accumulates quickly
- 50+ CLOSE_WAIT within 3-5 minutes

## Success Criteria
- [x] Identified CLOSE_WAIT accumulation
- [x] Found server not closing sockets
- [x] Understood TCP state machine issue
- [x] Proposed mitigation: Fix server to close sockets

## Autograder Scoring Rubric

### CLOSE_WAIT Identification (25 points)
- 25pts: Found CLOSE_WAIT connections with count
- 15pts: Mentioned connection issue without state
- 0pts: Did not identify CLOSE_WAIT

### Server Identification (25 points)
- 25pts: Found server process with CLOSE_WAIT
- 10pts: Mentioned server without evidence
- 0pts: Did not identify server

### Root Cause Analysis (30 points)
- 30pts: Explained server not closing sockets
- 20pts: Mentioned socket issue without explanation
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Fix server code to close sockets
- 10pts: Generic solution
- 0pts: No mitigation
