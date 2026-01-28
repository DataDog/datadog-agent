# Swap Thrashing

## Difficulty
Medium

## Problem Statement
System is constantly swapping due to memory pressure from processes consuming more RAM than available, causing severe performance degradation.

## Symptoms
Observable symptoms that an SRE would notice:
- High swap usage (>50% of swap space used)
- Frequent swap in/out activity visible in I/O stats
- System feels sluggish and unresponsive
- High iowait from swap device
- Available memory consistently near zero

## Root Cause
Multiple memory-hungry processes consuming more RAM than physically available, forcing the system to constantly swap pages between RAM and disk, creating a performance bottleneck.

## Investigation Steps
Recommended approach to diagnose:
1. **Check memory and swap**: Use `get_memory_info` to see high swap usage
2. **Check I/O stats**: Use `get_io_stats` to see swap device activity
3. **List memory hogs**: Use `list_processes` sorted by memory
4. **Sum memory usage**: Calculate total memory used by all processes
5. **Compare with RAM**: Verify total usage exceeds available RAM

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified high swap usage and thrashing
- Found multiple processes exceeding available RAM
- Root cause: Memory overcommitment causing swap thrashing

### Alternative Valid Diagnoses (Partial Credit)
- "System swapping heavily"
- "Not enough RAM for running processes"
- "Memory pressure causing performance issues"

### Key Terms (should be mentioned)
- "swap", "thrashing", "memory pressure"
- Swap usage percentage
- Total memory vs available RAM

### Common Errors (deductions)
- Confusing with single memory leak (-20%)
- Blaming disk I/O without mentioning swap (-25%)
- Not identifying memory overcommitment (-20%)

## Setup Instructions
```bash
cd /mcp/scenarios/swap-thrashing
python3 workload.py > /tmp/swap-thrashing.log 2>&1 &
echo $! > /tmp/swap-thrashing.pid
```

## Cleanup Instructions
```bash
if [ -f /tmp/swap-thrashing.pid ]; then
    kill $(cat /tmp/swap-thrashing.pid)
    rm /tmp/swap-thrashing.pid
fi
rm -f /tmp/swap-thrashing.log
```

## Expected Timeline
- **Short term** (3-5 minutes)
- Swap usage climbs to >50% within 3-5 minutes

## Success Criteria
- [x] Identified high swap usage
- [x] Found memory overcommitment
- [x] Understood root cause: Insufficient RAM
- [x] Proposed mitigation: Kill processes, add RAM, or reduce workload

## Autograder Scoring Rubric

### Swap Identification (25 points)
- 25pts: Identified swap thrashing with usage metrics
- 15pts: Mentioned swap without metrics
- 0pts: Did not identify swap issue

### Memory Overcommitment (25 points)
- 25pts: Showed total memory usage exceeds RAM
- 10pts: Mentioned high memory usage
- 0pts: Did not identify overcommitment

### Root Cause Analysis (30 points)
- 30pts: Explained memory overcommitment causing thrashing
- 20pts: Mentioned memory pressure
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Specific solution (kill processes, add RAM)
- 10pts: Generic solution
- 0pts: No mitigation
