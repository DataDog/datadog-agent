# File Secret Backends

## Supported Backends

The `datadog-secret-backend` utility currently supports the following file secret backends:

| Backend Type | File Type |
| :-- | :-- |
| [file.json](json.md) | [JSON](https://en.wikipedia.org/wiki/JSON) |
| [file.yaml](yaml.md) | [YAML](https://en.wikipedia.org/wiki/YAML) |


## File Permissions

The `datadog-secret-backend` file backend only requires read permissions from the local system Datadog Agent user (Linux: dd-agent; Windows: ddagentuser) to the configured JSON or YAML files.
