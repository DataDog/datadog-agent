# Disk Space Full

## Difficulty
Easy

## Problem Statement
A process is continuously writing large files to disk, rapidly consuming available disk space and potentially causing write failures for other services.

## Symptoms
Observable symptoms that an SRE would notice:
- Disk usage increasing rapidly toward 90-100%
- Large files accumulating in /tmp/scenario_logs/ directory
- "No space left on device" errors may appear in logs
- df command shows high disk usage percentage
- New files being created continuously

## Root Cause
An unchecked process writing large log files without any size limits, rotation, or cleanup mechanism. Files accumulate indefinitely until disk space is exhausted.

## Investigation Steps
Recommended approach to diagnose:
1. **Check disk usage**: Use `get_disk_usage` MCP tool or `bash_execute` with `df -h`
2. **Identify large directories**: Use `bash_execute` with `du -sh /tmp/* | sort -h`
3. **List files**: Check /tmp/scenario_logs/ for large/numerous files
4. **Find writing process**: Use `bash_execute` with `lsof +D /tmp/scenario_logs/` or check process list
5. **Confirm growth**: Monitor disk usage over time to confirm continued growth

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified process writing files to /tmp/scenario_logs/
- Resource issue: Disk space filling up (specific percentage)
- Root cause: Unbounded file writes without cleanup or rotation

### Alternative Valid Diagnoses (Partial Credit)
- "Disk filling up from log files"
- "Uncontrolled disk writes in /tmp"
- "Missing log rotation causing disk exhaustion"

### Key Terms (should be mentioned)
- "disk", "space", "full", "/tmp"
- "files", "writing", "logs"
- Process name or PID writing the files

### Common Errors (deductions)
- Only reporting disk usage without finding source (-40%)
- Confusing with memory issue (-25%)
- Not identifying specific directory or process (-30%)

## Setup Instructions
How to deploy the scenario:
```bash
cd /mcp/scenarios/disk-space-full
mkdir -p /tmp/scenario_logs
python3 workload.py > /tmp/disk-space-full.log 2>&1 &
echo $! > /tmp/disk-space-full.pid
```

## Cleanup Instructions
How to stop and clean up:
```bash
# Stop the process
if [ -f /tmp/disk-space-full.pid ]; then
    kill $(cat /tmp/disk-space-full.pid) 2>/dev/null
    rm /tmp/disk-space-full.pid
fi

# Clean up generated files
rm -rf /tmp/scenario_logs/
rm -f /tmp/disk-space-full.log
```

## Expected Timeline
How quickly the issue manifests:
- **Short term** (5-10 minutes)
- Depends on available disk space
- With ~40GB free, fills at rate of ~100MB/10sec = ~60 minutes to critical
- Can be configured to write faster if needed

## Success Criteria
What indicates a correct diagnosis:
- [x] Identified disk space filling up
- [x] Found /tmp/scenario_logs/ directory with large files
- [x] Identified the workload.py process writing the files
- [x] Understood root cause: No size limits or cleanup
- [x] Proposed mitigation: Stop process, clean up files, add rotation/limits

## Autograder Scoring Rubric

### Resource Identification (25 points)
- 25pts: Correctly identified disk space issue with specific percentage
- 15pts: Mentioned disk space without metrics
- 0pts: Identified wrong resource

### Source Identification (25 points)
- 25pts: Found /tmp/scenario_logs/ and identified writing process
- 15pts: Found directory but not process
- 5pts: Mentioned /tmp but didn't locate specific directory
- 0pts: Did not identify source

### Root Cause Analysis (30 points)
- 30pts: Explained unbounded writes without rotation/cleanup/limits
- 20pts: Mentioned uncontrolled writes but didn't explain lack of limits
- 10pts: Only described symptoms
- 0pts: No root cause analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (stop process, cleanup, add rotation)
- 10pts: Generic solution
- 0pts: No mitigation proposed
