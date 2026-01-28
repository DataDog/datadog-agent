# High CPU Usage

## Difficulty
Easy

## Problem Statement
A process is consuming excessive CPU resources, causing high system load and potential performance degradation for other services.

## Symptoms
Observable symptoms that an SRE would notice:
- Single process consistently consuming 90-100% of one CPU core
- System load average elevated (> 1.0 on single-core equivalent)
- Process visible at top of process list sorted by CPU usage
- High user CPU time in system metrics
- Process name: `python3` or `workload.py`

## Root Cause
A CPU-bound workload performing continuous cryptographic hashing (SHA256) operations without any sleep or yield, monopolizing CPU cycles.

## Investigation Steps
Recommended approach to diagnose:
1. **Check CPU metrics**: Use `get_cpu_info` MCP tool or `bash_execute` with `uptime` to see load average
2. **List top processes**: Use `list_processes` MCP tool or `bash_execute` with `ps aux --sort=-%cpu | head`
3. **Identify culprit**: Look for process with highest CPU percentage (should be ~100%)
4. **Get process details**: Use `get_process_info` with the PID to confirm it's the workload.py script
5. **Confirm diagnosis**: The process should be running continuously with no I/O wait, pure CPU consumption

## Acceptable Diagnoses
For the LLM autograder, list acceptable diagnostic conclusions:

### Primary Diagnosis (Full Credit)
- Identified process: `python3` or `workload.py` with specific PID
- Resource issue: CPU consumption at 90-100%
- Root cause: CPU-bound operation (hashing/computation) with no throttling or sleep

### Alternative Valid Diagnoses (Partial Credit)
- "High CPU usage by Python process"
- "Process consuming full CPU core"
- "CPU-intensive workload without rate limiting"

### Key Terms (should be mentioned)
- "CPU", "100%", "high load", "CPU-bound"
- Process name or PID
- "workload" or "python"

### Common Errors (deductions)
- Confusing with I/O wait (-15%): This is pure CPU, not I/O
- Blaming system/kernel (-20%): It's a user process
- Not identifying specific process (-30%): Must identify PID or name

## Setup Instructions
How to deploy the scenario:
```bash
cd /mcp/scenarios/high-cpu-usage
python3 workload.py > /tmp/high-cpu-usage.log 2>&1 &
echo $! > /tmp/high-cpu-usage.pid
```

Or using systemd:
```bash
# Copy workload to /opt/mcp-scenarios/
sudo mkdir -p /opt/mcp-scenarios
sudo cp workload.py /opt/mcp-scenarios/high-cpu-usage.py

# Create systemd service
sudo tee /etc/systemd/system/mcp-high-cpu.service > /dev/null <<EOF
[Unit]
Description=MCP Scenario: High CPU Usage

[Service]
Type=simple
ExecStart=/usr/bin/python3 /opt/mcp-scenarios/high-cpu-usage.py
Restart=always
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# Start service
sudo systemctl daemon-reload
sudo systemctl start mcp-high-cpu.service
```

## Cleanup Instructions
How to stop and clean up:
```bash
# If running as background process
if [ -f /tmp/high-cpu-usage.pid ]; then
    kill $(cat /tmp/high-cpu-usage.pid)
    rm /tmp/high-cpu-usage.pid
fi
rm -f /tmp/high-cpu-usage.log

# If running as systemd service
sudo systemctl stop mcp-high-cpu.service
sudo systemctl disable mcp-high-cpu.service
sudo rm /etc/systemd/system/mcp-high-cpu.service
sudo systemctl daemon-reload
```

## Expected Timeline
How quickly the issue manifests:
- **Immediate** (< 1 minute)
- CPU usage spikes to 100% within seconds of starting
- Symptoms are immediately observable

## Success Criteria
What indicates a correct diagnosis:
- [x] Identified the workload.py process by PID or name
- [x] Confirmed CPU usage at ~100% for this process
- [x] Understood root cause: CPU-bound computation without throttling
- [x] Proposed mitigation: Kill process, add rate limiting, or reduce computational intensity

## Autograder Scoring Rubric

### Process Identification (25 points)
- 25pts: Correctly identified process name and PID
- 15pts: Identified process name only
- 5pts: Mentioned high CPU but didn't identify process
- 0pts: Did not identify the process

### Resource Attribution (25 points)
- 25pts: Correctly identified CPU as the exhausted resource with specific percentage
- 15pts: Mentioned CPU but no specific metrics
- 0pts: Incorrect resource (e.g., memory, disk)

### Root Cause Analysis (30 points)
- 30pts: Explained it's a CPU-bound workload without throttling/sleep
- 20pts: Mentioned it's compute-intensive but didn't explain lack of throttling
- 10pts: Only mentioned symptoms without explaining cause
- 0pts: No root cause analysis

### Mitigation Proposal (20 points)
- 20pts: Proposed actionable solution (kill process, add throttling, limit CPU)
- 10pts: Generic solution without specifics
- 0pts: No mitigation proposed
