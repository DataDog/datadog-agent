# logs-agent

logs-agent collects logs and submits them to datadog's infrastructure.

## Structure

`logs` reads the config files, and instantiates what's needed.
Each log line comes from a source (such as file, network, docker), and then enters one of the available _pipeline - tailer|listener|container -> decoder -> processor -> strategy -> sender -> destination -> auditor_

`Tailer` tails a file and submits data to the processors

`Listener` listens on local network (TCP, UDP, Unix) and submits data to the processors

`Container` scans docker logs from stdout/stderr and submits data to the processors

`Decoder` converts bytes arrays into messages

`Processor` updates the messages, filtering, redacting, or adding metadata, and submits to the strategy.

`Strategy` converts a stream of messages into a stream of encoded payloads, batching if needed.

`Sender` submits the payloads to the destination(s).

`Destination` sends the actual encoded content to the intake, retries if needed, and sends successful payloads to the auditor.

`Auditor` notes that messages were properly submitted, and stores offsets for Agent restarts.

## Tests

```
# Run the unit tests
inv test --targets=./pkg/logs --timeout=10

```
