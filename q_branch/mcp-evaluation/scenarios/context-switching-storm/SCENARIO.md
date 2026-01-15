# Context Switching Storm

## Difficulty
Hard

## Problem Statement
Excessive context switches from many threads using tight synchronization primitives, causing high system CPU usage and performance degradation despite low actual work being done.

## Symptoms
Observable symptoms that an SRE would notice:
- Very high context switch rate (>100k/sec)
- High system CPU time but low user CPU time
- Load average high but actual CPU utilization moderate
- Many threads/processes in runnable state
- Poor application performance despite available CPU

## Root Cause
Many threads using condition variables, mutexes, or other synchronization primitives in tight loops, causing excessive thread wake-ups and context switches. Threads constantly yielding CPU to each other with minimal productive work.

## Investigation Steps
Recommended approach to diagnose:
1. **Check CPU info**: Use `get_cpu_info` or `bash_execute` with `vmstat 1` to see high context switches
2. **Check system CPU**: Look for high 'sy' (system) CPU time
3. **List processes**: Use `list_processes` to find process with many threads
4. **Monitor switches**: Use `bash_execute` with `pidstat -w` to see per-process context switches
5. **Check thread count**: Verify high thread count for process

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified very high context switch rate
- Found process with excessive thread synchronization
- Root cause: Lock contention or tight sync loops causing thrashing

### Alternative Valid Diagnoses (Partial Credit)
- "Context switching storm"
- "Excessive thread synchronization"
- "High system CPU from thread overhead"

### Key Terms (should be mentioned)
- "context switch", "thrashing", "synchronization"
- "threads", "locks", "contention"
- High system CPU time

### Common Errors (deductions)
- Confusing with CPU-bound workload (-25%)
- Not identifying context switch rate (-30%)
- Blaming scheduler without understanding cause (-20%)

## Setup Instructions
```bash
cd /mcp/scenarios/context-switching-storm
python3 workload.py > /tmp/context-switching.log 2>&1 &
echo $! > /tmp/context-switching.pid
```

## Cleanup Instructions
```bash
if [ -f /tmp/context-switching.pid ]; then
    kill $(cat /tmp/context-switching.pid)
    rm /tmp/context-switching.pid
fi
rm -f /tmp/context-switching.log
```

## Expected Timeline
- **Immediate** (< 1 minute)
- Context switch rate spikes immediately

## Success Criteria
- [x] Identified high context switch rate
- [x] Found process with excessive threading
- [x] Understood synchronization overhead
- [x] Proposed mitigation: Reduce threads, fix sync logic

## Autograder Scoring Rubric

### Context Switch Identification (25 points)
- 25pts: Found high context switch rate with numbers
- 15pts: Mentioned context switching without metrics
- 0pts: Did not identify context switches

### Process Identification (25 points)
- 25pts: Found process with threading issue
- 10pts: Mentioned process without details
- 0pts: Did not identify process

### Root Cause Analysis (30 points)
- 30pts: Explained excessive synchronization/lock contention
- 20pts: Mentioned threading issue without explanation
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (reduce threads, fix sync)
- 10pts: Generic solution
- 0pts: No mitigation
