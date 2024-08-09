E2E run command
===

This experimental tool can be used to run E2E snippets on Windows remote hosts.

# Examples

Run `agent status` command
```sh
go run ./tests/windows/cmd/run --host 10.1.59.199 agent cmd -- status
```

Install agent 7.51.1
```sh
WINDOWS_AGENT_VERSION=7.51.1-1 go run ./tests/windows/cmd/run --host 10.1.59.199 agent install -- 'TAGS="owner:myself"'
```

Uninstall the agent and delete the configuration directory
```sh
go run ./tests/windows/cmd/run --host 10.1.59.199 agent uninstall --remove-config
```
