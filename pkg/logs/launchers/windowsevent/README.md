# How to setup a windows eventlog dev environment

Cross compilation from mac is not easy, an easier path is to use a linux vm for that (for instance `ubuntu/trusty64` vagrant vm).
Requirements are to install go 1.21+, and to install `mingw-w64` with apt.

Once those requirements are met, to build, run:
```
GOOS=windows CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -mod=mod -tags "log" -o ./DataDog/datadog-agent/bin/agent/agent github.com/DataDog/datadog-agent/cmd/agent
```

The binary can then be run in a windows vm, for instance `opentable/win-2012r2-standard-amd64-nocm`:

```
agent.exe start -c agent-conf\datadog.yaml
```
