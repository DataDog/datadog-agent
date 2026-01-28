# Zombie Processes

## Difficulty
Easy

## Problem Statement
A parent process is spawning child processes that exit but are not being reaped, leaving zombie processes accumulating in the system.

## Symptoms
Observable symptoms that an SRE would notice:
- Multiple processes in "Z" (zombie) state in process list
- Zombie count increasing over time
- Process state shows as "<defunct>" or "Z"
- Parent process still running
- Zombies have minimal resource usage but consume PID slots

## Root Cause
Parent process spawning children with subprocess but not calling wait(), waitpid(), or properly handling child exit. Zombies remain until parent reaps them or parent exits.

## Investigation Steps
Recommended approach to diagnose:
1. **List processes**: Use `list_processes` MCP tool to see processes in Z state
2. **Count zombies**: Use `bash_execute` with `ps aux | grep defunct | wc -l`
3. **Find parent**: Use `get_process_info` on zombie PID to find PPID (parent PID)
4. **Identify parent process**: Check parent process details
5. **Monitor growth**: Confirm zombie count increases over time

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified multiple zombie processes (Z state or <defunct>)
- Found parent process not reaping children
- Root cause: Parent not calling wait/waitpid

### Alternative Valid Diagnoses (Partial Credit)
- "Zombie processes accumulating"
- "Parent not reaping child processes"
- "<defunct> processes growing"

### Key Terms (should be mentioned)
- "zombie", "Z state", "defunct"
- "parent", "reap", "wait"
- Parent process PID or name

### Common Errors (deductions)
- Trying to kill zombies directly (-20%): Can't kill zombies, must fix parent
- Not identifying parent process (-30%)
- Confusing with orphan processes (-15%)

## Setup Instructions
How to deploy the scenario:
```bash
cd /mcp/scenarios/zombie-processes
python3 workload.py > /tmp/zombie-processes.log 2>&1 &
echo $! > /tmp/zombie-processes.pid
```

## Cleanup Instructions
How to stop and clean up:
```bash
# Stop parent process (this will reap all zombies automatically)
if [ -f /tmp/zombie-processes.pid ]; then
    kill $(cat /tmp/zombie-processes.pid)
    rm /tmp/zombie-processes.pid
fi
rm -f /tmp/zombie-processes.log
```

## Expected Timeline
How quickly the issue manifests:
- **Short term** (3-5 minutes)
- Spawns child every 5 seconds = 12 zombies/minute
- Should have 20-50 zombies within 5 minutes

## Success Criteria
What indicates a correct diagnosis:
- [x] Identified zombie processes in Z state
- [x] Found parent process responsible
- [x] Understood root cause: Parent not reaping children
- [x] Proposed mitigation: Fix parent to call wait(), or kill parent

## Autograder Scoring Rubric

### Zombie Identification (25 points)
- 25pts: Identified zombies with count and state (Z or <defunct>)
- 15pts: Mentioned zombies without specifics
- 0pts: Did not identify zombies

### Parent Identification (25 points)
- 25pts: Found parent process (PID and name)
- 10pts: Mentioned parent concept but didn't identify
- 0pts: Did not identify parent

### Root Cause Analysis (30 points)
- 30pts: Explained parent not reaping with wait/waitpid
- 20pts: Mentioned parent not cleaning up children
- 10pts: Only described symptoms
- 0pts: No analysis

### Mitigation Proposal (20 points)
- 20pts: Correct solution (fix parent code or kill parent)
- 10pts: Incorrect solution (trying to kill zombies directly)
- 0pts: No mitigation
