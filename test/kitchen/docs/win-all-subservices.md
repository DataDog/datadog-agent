## win-all-subservices.md

### Objective

Test the Windows command line installer options for explicitly enabling the various datadog subservices (processes, trace, and logs)

### Installation Recipe

Uses the \_install_windows_base recipe to do a fresh install of the agent.
1. Uses a fresh install because install options are ignored on upgrades
2. Uses the \_base recipe which allows setting of additional options

### Test definition

The test is successful if:
1. APM 
    1. APM is enabled in the configuration file
    2. Something is listening on port 8126
    3. there is a process in the process table called `trace-agent.exe`
    4. There is a service registered called `datadog-trace-agent` which can be queried via `sc qc`
2. Logs
    1. Logs is enabled in the configuration file
3. Process
    1. Process is enabled in the configuration file
    2. There is a process in the process table called `process-agent.exe`
    3. There is a service registered called `datadog-process-agent` which can be queried via `sc qc`
   