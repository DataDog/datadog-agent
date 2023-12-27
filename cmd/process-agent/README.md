# Datadog Process Agent

## Installation

See the [Live Processes docs](https://docs.datadoghq.com/graphing/infrastructure/process/#installation) for installation instructions.

## Development or running from source

Pre-requisites:

* `go >= 1.21`
* `invoke`

Check out the repo in your `$GOPATH`

```
cd $GOPATH/src/github.com/DataDog
git clone git@github.com:DataDog/datadog-agent.git
cd datadog-agent
```

Note that you must be in `$GOPATH/src/github.com/DataDog/datadog-agent`, NOT `~/dd/datadog-agent`.

To build the Process Agent run:

```
inv -e process-agent.build
```

You can now run the Agent on the command-line:

```
./bin/process-agent/process-agent -config $PATH_TO_PROCESS_CONFIG_FILE
```
