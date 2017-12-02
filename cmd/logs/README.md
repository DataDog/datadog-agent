# datadog-log-agent

datadog-log-agent collects logs and submits them into datadog's infrastructure.
This repository is temporary, mid term plan is to merge the code with [datadog-agent](https://github.com/DataDog/datadog-agent)

Contributions today may not be accepted, as plan is to have this code merged into datadog-agent repository

## Structure

`logagent` reads the config files, and instanciates what's needed.
Each log line comes from a source (e.g. file, network), and then enters one of the available pipeline - _decoder -> processor -> sender -> auditor_

`Tailer` tails a file and submits data to the processors

`Listener` listens on local network and submits data to the processors

`Decoder` converts bytes arrays into messages

`Processor` updates the messages, filtering, redacting or adding metadata, and submits to the forwarder

`Forwarder` submits the messages to the intake, and notifies the auditor

`Auditor` notes that messages were properly submitted, stores offsets for agent restarts

## How to run

- `rake deps`
- `rake build`
- setup config files
- `./build/logagent --ddconfig pkg/logagent/etc/datadog.yaml --ddconfd pkg/logagent/etc/conf.d/`
