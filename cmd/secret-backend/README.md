# datadog-secret-backend

[![.github/workflows/release.yaml](https://github.com/DataDog/datadog-secret-backend/actions/workflows/release.yaml/badge.svg)](https://github.com/DataDog/datadog-secret-backend/actions/workflows/release.yaml)

> **datadog-secret-backend** is an implementation of the [Datadog Agent Secrets Management](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) executable supporting multiple backend secret providers.

**IMPORTANT NOTE**: A new major version, `v1`, of `datadog-secret-backend` has been released with a simplified configuration process and a better integration with the Datadog Agent. This new version is not compatible with the `v0` configuration files. `v0` will continue to be maintained on the `v0` branch of this repository and remains compatible with the Datadog Agent. You can find previous releases of `v0` [here](https://github.com/DataDog/datadog-secret-backend/releases).

The `v1` version comes with the following key improvements:
1. The `datadog-secret-backend` is now shipped within the Datadog Agent starting with version 7.69.
2. The backend is now configured directly within the configuration file of the Datadog Agent rather than a dedicated external file.
3. The type of backend is now configured directly from the configuration file of the Datadog Agent. There is no need to prefix your secret with the `backendID`.
More information on how to use the `v1` can be found [here](https://docs.datadoghq.com/agent/configuration/secrets-management).

## Supported Backends

| Backend | Provider | Description |
| :-- | :-- | :-- |
| [aws.secrets](docs/aws/secrets.md) | [aws](docs/aws/README.md) | Datadog secrets in AWS Secrets Manager |
| [aws.ssm](docs/aws/ssm.md) | [aws](docs/aws/README.md) | Datadog secrets in AWS Systems Manager Parameter Store |
| [azure.keyvault](docs/azure/keyvault.md) | [azure](docs/azure/README.md) | Datadog secrets in Azure Key Vault |
| [hashicorp.vault](docs/hashicorp/vault.md) | [hashicorp](docs/hashicorp/README.md) | Datadog secrets in Hashicorp Vault |
| [file.json](docs/file/json.md) | [file](docs/file/README.md) | Datadog secrets in local JSON files|
| [file.yaml](docs/file/yaml.md) | [file](docs/file/README.md) | Datadog secrets in local YAML files|

## Usage

Reference each supported backend type's documentation on specific usage examples and configuration options.

## License

[BSD-3-Clause License](LICENSE)
