# Datadog Process Agent

## Installation

See the [Live Processes docs](https://docs.datadoghq.com/graphing/infrastructure/process/#installation) for installation instructions.

## Development or running from source

Pre-requisites:

* `go >= 1.12`
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

## Development
The easiest way to build and test is inside a Vagrant VM. You can provision the VM by running `./pkg/process/tools/dev_setup.sh` and SSHing into the VM with `vagrant ssh` (vagrant must be installed.)

The VM will mount your local $GOPATH, so you can edit source code with your editor of choice.

For development on the system-probe, `rake ebpf:nettop` will run a small testing program which periodically prints statistics about TCP/UDP traffic inside the VM.
