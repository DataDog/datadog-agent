# File Descriptor Leak

## Difficulty
Medium

## Problem Statement
A process is opening files without closing them, leading to file descriptor exhaustion and eventual "Too many open files" errors.

## Symptoms
Observable symptoms that an SRE would notice:
- Process file descriptor count steadily increasing
- "Too many open files" errors in logs (OSError: [Errno 24])
- Process eventually unable to open new files or sockets
- FD count approaches ulimit (typically 1024 for non-root)

## Root Cause
Application opening files (or sockets) without properly closing them via close(). File descriptors accumulate until hitting process ulimit.

## Investigation Steps
Recommended approach to diagnose:
1. **Check process info**: Use `get_process_info` to see FD count
2. **Monitor growth**: Check FD count multiple times to confirm growth
3. **Check ulimit**: Use `bash_execute` with `ulimit -n` to see limit
4. **List open files**: Use `bash_execute` with `lsof -p <pid>` to see what's open
5. **Check for errors**: Look for "Too many open files" in logs

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified process with growing FD count
- Found FD leak (files not being closed)
- Root cause: Missing close() calls on file handles

### Alternative Valid Diagnoses (Partial Credit)
- "File descriptor leak"
- "Too many open files error"
- "Process hitting FD limit"

### Key Terms (should be mentioned)
- "file descriptor", "FD", "leak"
- "not closed" or "missing close"
- FD count trend

### Common Errors (deductions)
- Confusing with disk space issue (-20%)
- Not checking FD count growth (-25%)
- Blaming ulimit without identifying leak (-20%)

## Setup Instructions
```bash
cd /mcp/scenarios/file-descriptor-leak
python3 workload.py > /tmp/fd-leak.log 2>&1 &
echo $! > /tmp/fd-leak.pid
```

## Cleanup Instructions
```bash
if [ -f /tmp/fd-leak.pid ]; then
    kill $(cat /tmp/fd-leak.pid)
    rm /tmp/fd-leak.pid
fi
rm -f /tmp/fd-leak.log
```

## Expected Timeline
- **Short term** (2-3 minutes)
- Opens 10 FDs/second = 600/minute
- Hits ulimit (1024) in ~2 minutes

## Success Criteria
- [x] Identified FD leak with growing count
- [x] Found process responsible
- [x] Understood root cause: Files not closed
- [x] Proposed mitigation: Fix code to close files or restart

## Autograder Scoring Rubric

### FD Growth Identification (25 points)
- 25pts: Showed FD count growing with measurements
- 15pts: Mentioned high FD count without trend
- 0pts: Did not identify FD issue

### Process Identification (25 points)
- 25pts: Found process with FD leak
- 10pts: Mentioned process without specifics
- 0pts: Did not identify process

### Root Cause Analysis (30 points)
- 30pts: Explained files not being closed
- 20pts: Mentioned FD leak without explanation
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (fix code, restart)
- 10pts: Generic solution
- 0pts: No mitigation
