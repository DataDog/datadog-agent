# ```agent diagnose``` CLI command

```agent diagnose``` command is one of the Agent CLI commands ([agent other commands](https://docs.datadoghq.com/agent/guide/agent-commands/?tab=agentv6v7#other-commands)). It runs internal self-diagnostic tests and outputs their results and if problems are found report them (and in some cases remediation steps).

## ```diagnose``` command line options
List sub-commands:
```
agent diagnose -h
Validate Agent installation, configuration and environment

Usage:
  agent.exe diagnose [flags]
  agent.exe diagnose [command]

Available Commands:
  all                   Validate Agent installation, configuration and environment
  datadog-connectivity  Check connectivity between your system and Datadog endpoints
  metadata-availability Check availability of cloud provider and container metadata endpoints
  show-metadata         Print metadata payloads sent by the agent
```
## ```diagnose all``` sub-command
### Options
List all sub-command options:
```
agent.exe diagnose all -h
Validate Agent installation, configuration and environment

Usage:
  agent.exe diagnose all [flags]

Flags:
  -e, --exclude strings   diagnose suites not to run as a list of regular expressions
  -h, --help              help for all
  -i, --include strings   diagnose suites to run as a list of regular expressions
  -t, --list              list diagnose suites
  -l, --local             force diagnose execution by the command line instead of the agent process (useful when troubleshooting privilege related problems)
  -v, --verbose           verbose output, includes passed diagnoses, and diagnoses description
  -j, --json              output in a json file
```

### ```include``` and ```exclude``` options
agent diagnose --include and or --exclude options allow to filter to specific diagnose suites

### ```list``` option
List names of all registered diagnose suites. Output also will be filtered if include and or exclude options are specified.

### ```verbose``` option
Normally a successful diagnosis is printed as a single dot character. If verbose option is specified successful diagnosis is printed fully. With verbose option diagnosis description is also printed.

### ```local``` option
Normally internal diagnose functions will run in the context of agent and other services. It can be overridden via –run-as-user options and if specified diagnose functions will be executed in context of the agent diagnose CLI process if possible.

## ```json``` option
Normally diagnose is displayed on stdout. If JSON option is specified, the output will be formated as JSON
