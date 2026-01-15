# Memory Leak

## Difficulty
Medium

## Problem Statement
A service with an unbounded cache is continuously consuming memory without eviction, leading to gradual memory exhaustion and potential OOM kill.

## Symptoms
Observable symptoms that an SRE would notice:
- Process RSS (Resident Set Size) growing steadily over time
- System available memory decreasing
- Memory usage increasing linearly without plateau
- Eventually may trigger OOM killer (exit code 137 or kernel log entry)
- No corresponding increase in workload or requests

## Root Cause
In-memory cache adding entries without any eviction policy, size limit, or TTL. Cache grows unbounded as new items are added but old items are never removed.

## Investigation Steps
Recommended approach to diagnose:
1. **Check system memory**: Use `get_memory_info` to see available memory decreasing
2. **List top memory consumers**: Use `list_processes` sorted by memory
3. **Identify growing process**: Use `get_process_info` with PID, check RSS over time
4. **Monitor growth rate**: Take multiple measurements to confirm linear growth
5. **Check for OOM**: Use `bash_execute` with `dmesg | grep -i oom` or check /var/log/kern.log

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified process with growing memory (specific PID and RSS trend)
- Resource issue: Memory leak/unbounded growth
- Root cause: Cache without eviction policy or size limits

### Alternative Valid Diagnoses (Partial Credit)
- "Memory leak in Python process"
- "Unbounded memory growth"
- "Process consuming all available memory"

### Key Terms (should be mentioned)
- "memory leak", "growing", "unbounded"
- "cache", "no eviction"
- Process RSS or memory usage trend

### Common Errors (deductions)
- Confusing with disk space issue (-25%)
- Not identifying growth trend, only snapshot (-20%)
- Blaming system/kernel for memory usage (-20%)

## Setup Instructions
How to deploy the scenario:
```bash
cd /mcp/scenarios/memory-leak
python3 workload.py > /tmp/memory-leak.log 2>&1 &
echo $! > /tmp/memory-leak.pid
```

## Cleanup Instructions
How to stop and clean up:
```bash
if [ -f /tmp/memory-leak.pid ]; then
    kill $(cat /tmp/memory-leak.pid)
    rm /tmp/memory-leak.pid
fi
rm -f /tmp/memory-leak.log
```

## Expected Timeline
How quickly the issue manifests:
- **Long term** (15-30 minutes)
- Growth rate: ~30MB/minute
- Takes 15-30 minutes to reach 500MB-1GB depending on starting point

## Success Criteria
What indicates a correct diagnosis:
- [x] Identified process with memory leak
- [x] Documented memory growth trend (before/after measurements)
- [x] Understood root cause: Unbounded cache
- [x] Proposed mitigation: Add eviction policy, size limits, or restart service

## Autograder Scoring Rubric

### Memory Growth Identification (25 points)
- 25pts: Showed memory growing over time with measurements
- 15pts: Identified high memory usage without trend
- 0pts: Did not identify memory issue

### Process Identification (25 points)
- 25pts: Found process (PID and name) with growing RSS
- 10pts: Mentioned process without specifics
- 0pts: Did not identify process

### Root Cause Analysis (30 points)
- 30pts: Explained unbounded cache/data structure without eviction
- 20pts: Mentioned memory leak but didn't explain cause
- 10pts: Only reported symptoms
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (add eviction, size limits, fix code, restart)
- 10pts: Generic solution
- 0pts: No mitigation
