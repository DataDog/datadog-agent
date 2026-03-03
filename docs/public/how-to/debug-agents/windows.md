# Windows Troubleshooting

Prior to 7.23, Agent binaries (Datadog Agent, Process Agent, Trace Agent, etc.) on Windows contain symbol information.

Starting from 7.23, Agent binaries on Windows have debugging information stripped. The original files are packed in a
file called debug package.

## Prerequisite

To debug Agent process, Golang Runtime, Git and Golang Delve must be installed.

Download the matching debug package. If the MSI file is `datadog-agent-7.23.0-x86_64.msi`, the debug package should be
`datadog-agent-7.23.0-x86_64.debug.zip`.

## Live Debugging

Delve debugger on Windows cannot attach to the service process. The corresponding Windows service must be stopped and
disabled.

For pre 7.23, start the Agent executable in the interactive session.

For 7.23 or later version, find the file in the debug package. For `agent.exe`, the file in debug package is under
`\src\datadog-agent\src\github.com\DataDog\datadog-agent\bin\agent\agent.exe.debug`. You might find the same file under
`\omnibus-ruby\src\cf-root\bin`. Use either one is fine. Copy the file to replace the executable file you want to debug,
start the agent executable in the interactive session.

Use `dlv attach PID` to attach to the running process and start debugging.

## Non-live Debugging

Use `dlv core DUMPFILE EXEFILE` to debug against a dump file.

For 7.23 or newer, the EXEFILE is the .debug file in the debug package.
