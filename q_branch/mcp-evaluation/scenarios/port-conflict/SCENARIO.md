# Port Conflict

## Difficulty
Easy

## Problem Statement
Multiple services attempting to bind to the same network port, causing one to fail with an "Address already in use" error.

## Symptoms
Observable symptoms that an SRE would notice:
- Service failing to start with bind/address error
- Error messages: "Address already in use" or "OSError: [Errno 98]"
- Port 8080 showing as already bound/listening
- One process successfully listening, another crashing or in restart loop
- Duplicate service configurations

## Root Cause
Two services configured to use the same port (8080). The first service to start binds successfully, the second fails because the port is already occupied.

## Investigation Steps
Recommended approach to diagnose:
1. **Check listening ports**: Use `get_listening_ports` MCP tool or `bash_execute` with `ss -tlnp` or `netstat -tlnp`
2. **Find port 8080**: Look for which process is listening on port 8080
3. **Check process logs**: Use `read_file` or `tail_file` on log files to see bind errors
4. **List processes**: Use `find_process` or `list_processes` to find both service instances
5. **Confirm conflict**: Verify two processes trying to use the same port

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified both processes attempting to use port 8080
- Resource issue: Port 8080 already in use
- Root cause: Port conflict between two services

### Alternative Valid Diagnoses (Partial Credit)
- "Two services on same port"
- "Port 8080 bind conflict"
- "Address already in use error"

### Key Terms (should be mentioned)
- "port", "8080", "conflict", "bind"
- "address already in use" or similar error
- Both process names or PIDs

### Common Errors (deductions)
- Only identifying one process (-30%)
- Confusing with network connectivity issue (-20%)
- Not specifying the port number (-15%)

## Setup Instructions
How to deploy the scenario:
```bash
cd /mcp/scenarios/port-conflict
python3 server1.py > /tmp/port-conflict-1.log 2>&1 &
echo $! > /tmp/port-conflict-1.pid
sleep 2
python3 server2.py > /tmp/port-conflict-2.log 2>&1 &
echo $! > /tmp/port-conflict-2.pid
```

## Cleanup Instructions
How to stop and clean up:
```bash
# Stop both processes
for pidfile in /tmp/port-conflict-*.pid; do
    if [ -f "$pidfile" ]; then
        kill $(cat "$pidfile") 2>/dev/null
        rm "$pidfile"
    fi
done

rm -f /tmp/port-conflict-*.log
```

## Expected Timeline
How quickly the issue manifests:
- **Immediate** (< 1 minute)
- Second server fails immediately on startup
- Error visible in logs within seconds

## Success Criteria
What indicates a correct diagnosis:
- [x] Identified both processes attempting to use port 8080
- [x] Found "Address already in use" error in logs
- [x] Understood root cause: Port conflict
- [x] Proposed mitigation: Change port for one service or stop duplicate

## Autograder Scoring Rubric

### Process Identification (25 points)
- 25pts: Identified both processes (names and/or PIDs)
- 15pts: Identified only one process
- 0pts: Did not identify processes

### Resource Attribution (25 points)
- 25pts: Correctly identified port 8080 conflict
- 15pts: Mentioned port conflict without specifics
- 0pts: Incorrect resource issue

### Root Cause Analysis (30 points)
- 30pts: Explained port conflict between two services
- 20pts: Mentioned bind error but didn't explain conflict
- 10pts: Only reported error message
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (stop one, reconfigure port)
- 10pts: Generic solution
- 0pts: No mitigation
