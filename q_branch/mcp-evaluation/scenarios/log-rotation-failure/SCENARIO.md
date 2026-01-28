# Log Rotation Failure

## Difficulty
Medium

## Problem Statement
Log rotation is not working, causing a single log file to grow unbounded and consume excessive disk space.

## Symptoms
Observable symptoms that an SRE would notice:
- Single log file growing to hundreds of MB or GB
- No rotated log files (no .1, .2, .gz archives)
- Disk usage increasing from one large file
- Recent disk space decrease correlates with log growth
- Logrotate not running or misconfigured

## Root Cause
Log rotation mechanism (logrotate) either not configured, misconfigured, or not running for this service's logs. Without rotation, logs accumulate indefinitely in a single file.

## Investigation Steps
Recommended approach to diagnose:
1. **Check disk usage**: Use `get_disk_usage` to see space consumed
2. **Find large files**: Use `bash_execute` with `du -sh /tmp/*` or search /tmp
3. **Check log file size**: Use `bash_execute` with `ls -lh` on log directory
4. **Look for rotation**: Check for .1, .2, .gz files (rotated logs)
5. **Check logrotate config**: Use `read_file` on logrotate config if present

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified large log file growing unbounded
- No rotated log files present
- Root cause: Log rotation not working or not configured

### Alternative Valid Diagnoses (Partial Credit)
- "Log file too large"
- "Missing log rotation"
- "Unbounded log growth"

### Key Terms (should be mentioned)
- "log", "rotation", "unbounded"
- File size or growth rate
- "logrotate" or "no rotation"

### Common Errors (deductions)
- Confusing with disk filling from data files (-20%)
- Not checking for rotated files (-25%)
- Blaming application logging verbosity (-15%)

## Setup Instructions
How to deploy the scenario:
```bash
cd /mcp/scenarios/log-rotation-failure
mkdir -p /tmp/app_logs
python3 workload.py > /tmp/log-rotation-failure.log 2>&1 &
echo $! > /tmp/log-rotation-failure.pid
```

## Cleanup Instructions
How to stop and clean up:
```bash
if [ -f /tmp/log-rotation-failure.pid ]; then
    kill $(cat /tmp/log-rotation-failure.pid)
    rm /tmp/log-rotation-failure.pid
fi
rm -f /tmp/log-rotation-failure.log
rm -rf /tmp/app_logs/
```

## Expected Timeline
How quickly the issue manifests:
- **Short term** (10 minutes)
- Growth rate: ~1MB/minute
- Should reach 10-20MB within 10-15 minutes

## Success Criteria
What indicates a correct diagnosis:
- [x] Identified large log file
- [x] Confirmed no rotation occurring (no .1, .2, .gz files)
- [x] Understood root cause: Missing/broken log rotation
- [x] Proposed mitigation: Configure logrotate or implement rotation

## Autograder Scoring Rubric

### Log File Identification (25 points)
- 25pts: Found large log file with size details
- 15pts: Mentioned large file without specifics
- 0pts: Did not identify log file

### Rotation Check (25 points)
- 25pts: Confirmed no rotation (no .1, .2, .gz files)
- 10pts: Mentioned rotation but didn't verify
- 0pts: Did not check rotation

### Root Cause Analysis (30 points)
- 30pts: Explained missing/broken log rotation
- 20pts: Mentioned rotation issue without details
- 10pts: Only reported large file
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (configure logrotate, add rotation script)
- 10pts: Generic solution
- 0pts: No mitigation
