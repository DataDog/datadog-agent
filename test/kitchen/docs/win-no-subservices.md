## win-no-subservices.md

### Objective

Test the Windows command line installer options for explicitly disabling the various datadog subservices (processes, trace, and logs)

### Installation Recipe

Uses the \_install_windows_base recipe to do a fresh install of the agent.
1. Uses a fresh install because install options are ignored on upgrades
2. Uses the \_base recipe which allows setting of additional options

### Test definition

The test is successful if:
1. APM 
    1. APM is disabled in the configuration file
    2. Nothing is listening on port 8126
    3. there is no process in the process table called `trace-agent.exe`
    4. There is not a service registered called `datadog-trace-agent` which can be queried via `sc qc`
2. Logs
    1. Logs is not enabled in the configuration file
3. Process
    1. Process is disabled in the configuration file
    2. There is no process in the process table called `process-agent.exe`
    3. There is not a service registered called `datadog-process-agent` which can be queried via `sc qc`
   