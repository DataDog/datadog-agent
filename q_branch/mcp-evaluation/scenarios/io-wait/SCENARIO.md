# I/O Wait

## Difficulty
Hard

## Problem Statement
Multiple processes performing synchronous disk I/O simultaneously, creating I/O bottleneck and high iowait, causing system slowdown despite low CPU usage.

## Symptoms
Observable symptoms that an SRE would notice:
- High iowait percentage in CPU stats (>20%)
- Low CPU user/system time but system feels slow
- Multiple processes in 'D' (uninterruptible sleep) state
- High disk I/O statistics (reads/writes per second)
- Load average high but CPU utilization low

## Root Cause
Multiple processes doing synchronous disk writes with fsync() simultaneously, contending for disk I/O bandwidth. Each process blocks waiting for disk, creating I/O bottleneck.

## Investigation Steps
Recommended approach to diagnose:
1. **Check CPU info**: Use `get_cpu_info` to see high iowait
2. **Check I/O stats**: Use `get_io_stats` to see high disk activity
3. **List processes**: Use `list_processes` to find processes in 'D' state
4. **Identify writers**: Use `bash_execute` with `iotop` or check /proc/<pid>/io
5. **Confirm sync writes**: Multiple processes doing heavy I/O

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified high iowait with low CPU usage
- Found multiple processes doing synchronous disk I/O
- Root cause: Disk I/O bottleneck from contention

### Alternative Valid Diagnoses (Partial Credit)
- "High iowait causing slowdown"
- "Disk I/O bottleneck"
- "Processes waiting on disk"

### Key Terms (should be mentioned)
- "iowait", "I/O", "disk"
- "synchronous" or "fsync"
- Multiple processes doing I/O

### Common Errors (deductions)
- Confusing with CPU bottleneck (-25%)
- Blaming single process (-20%)
- Not identifying I/O contention (-20%)

## Setup Instructions
```bash
cd /mcp/scenarios/io-wait
python3 workload.py > /tmp/io-wait.log 2>&1 &
echo $! > /tmp/io-wait.pid
```

## Cleanup Instructions
```bash
if [ -f /tmp/io-wait.pid ]; then
    kill $(cat /tmp/io-wait.pid)
    rm /tmp/io-wait.pid
fi
rm -f /tmp/io-wait.log
rm -f /tmp/io_test_*.dat
```

## Expected Timeline
- **Immediate** (< 1 minute)
- iowait spikes immediately when processes start writing

## Success Criteria
- [x] Identified high iowait
- [x] Found multiple processes doing disk I/O
- [x] Understood I/O contention bottleneck
- [x] Proposed mitigation: Reduce I/O, stagger writes, faster disk

## Autograder Scoring Rubric

### IOWait Identification (25 points)
- 25pts: Found high iowait with percentage
- 15pts: Mentioned I/O issue without metrics
- 0pts: Did not identify iowait

### Process Identification (25 points)
- 25pts: Found multiple processes doing I/O
- 10pts: Mentioned I/O processes without specifics
- 0pts: Did not identify processes

### Root Cause Analysis (30 points)
- 30pts: Explained I/O contention from multiple writers
- 20pts: Mentioned I/O bottleneck without explanation
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (reduce I/O, stagger, faster disk)
- 10pts: Generic solution
- 0pts: No mitigation
