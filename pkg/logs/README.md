# logs-agent

logs-agent collects logs and submits them to datadog's infrastructure.

## Structure

`logs` reads the config files, and instanciates what's needed.
Each log line comes from a source (e.g. file, network, docker), and then enters one of the available _pipeline - decoder -> processor -> sender -> auditor_

`Tailer` tails a file and submits data to the processors

`Listener` listens on local network and submits data to the processors

`Container` scans docker logs from stdout/stderr and submits data to the processors

`Decoder` converts bytes arrays into messages

`Processor` updates the messages, filtering, redacting or adding metadata, and submits to the forwarder

`Forwarder` submits the messages to the intake, and notifies the auditor

`Auditor` notes that messages were properly submitted, stores offsets for agent restarts
