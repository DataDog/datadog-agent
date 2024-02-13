# datadog-secret-backend

[![.github/workflows/release.yaml](https://github.com/rapdev-io/datadog-secret-backend/actions/workflows/release.yaml/badge.svg)](https://github.com/rapdev-io/datadog-secret-backend/actions/workflows/release.yaml)

> **datadog-secret-backend** is an implementation of the [Datadog Agent Secrets Management](https://docs.datadoghq.com/agent/guide/secrets-management/?tab=linux) executable supporting multiple backend secret providers.

## Supported Backends

| Backend | Provider | Description |
| :-- | :-- | :-- |
| [aws.secrets](docs/aws/secrets.md) | [aws](docs/aws/README.md) | Datadog secrets in AWS Secrets Manager |
| [aws.ssm](docs/aws/ssm.md) | [aws](docs/aws/README.md) | Datadog secrets in AWS Systems Manager Parameter Store |
| [azure.keyvault](docs/azure/keyvault.md) | [azure](docs/azure/README.md) | Datadog secrets in Azure Key Vault |
| [hashicorp.vault](docs/hashicorp/vault.md) | [hashicorp](docs/hashicorp/README.md) | Datadog secrets in Hashicorp Vault |
| [file.json](docs/file/json.md) | [file](docs/file/README.md) | Datadog secrets in local JSON files|
| [file.yaml](docs/file/yaml.md) | [file](docs/file/README.md) | Datadog secrets in local YAML files|

## Installation

1. Make a new folder in `/etc/` to hold all the files required for this module in one place:

    ```
    ## Linux
    mkdir -p /etc/rapdev-datadog

    ## Windows
    mkdir 'C:\Program Files\rapdev-datadog\'
    ```

2. Download the most recent version of the secret backend module by hitting the latest release endpoint from the `rapdev-io` repo by running one of the commands below:

    ```
    ## Linux (amd64)
    curl -L https://github.com/rapdev-io/datadog-secret-backend/releases/latest/download/datadog-secret-backend-linux-amd64.tar.gz \ 
    -o /tmp/datadog-secret-backend-linux-amd64.tar.gz

    ## Linux (386)
    curl -L https://github.com/rapdev-io/datadog-secret-backend/releases/latest/download/datadog-secret-backend-linux-386.tar.gz \ 
    -o /tmp/datadog-secret-backend-linux-386.tar.gz

    ## Windows (amd64)
    Invoke-WebRequest https://github.com/rapdev-io/datadog-secret-backend/releases/latest/download/datadog-secret-backend-windows-amd64.zip -OutFile 'C:\Program Files\rapdev-datadog\datadog-secret-backend-windows-amd64.zip'

    ## Windows (386)
    Invoke-WebRequest https://github.com/rapdev-io/datadog-secret-backend/releases/latest/download/datadog-secret-backend-windows-386.zip -OutFile 'C:\Program Files\rapdev-datadog\datadog-secret-backend-windows-386.zip'
    ```

3. Once you have the file from the github repo, you'll need to unzip it to get the executable:

    ```
    ## Linux (amd64, change end of filename to "386" if needed)
    tar -xvzf /tmp/datadog-secret-backend-linux-amd64.tar.gz \
    -C /etc/rapdev-datadog

    ## Windows (amd64, change end of filename to "386" if needed)
    Expand-Archive -LiteralPath 'C:\Program Files\rapdev-datadog\datadog-secret-backend-windows-amd64.zip' -DestinationPath 'C:\Program Files\rapdev-datadog\'
    ```

4. (Optional) Remove the old tar'd file:

    ```
    ## Linux
    rm /tmp/datadog-secret-backend-linux-amd64.tar.gz

    ## Windows
    Remove-Item 'C:\Program Files\rapdev-datadog\datadog-secret-backend-windows-amd64.zip'
    ```

5. Update the executable to have the required permissions. Datadog agent expects the executable to only be used by the `dd-agent` user for Linux and `ddagentuser` for Windows.

    ```
    ## Linux
    chown dd-agent:root /etc/rapdev-datadog/datadog-secret-backend
    chmod 500 /etc/rapdev-datadog/datadog-secret-backend

    ## Windows
    1) Right click on the "datadog-secret-backend.exe" and select "Properties".
    2) Click on the Security tab.
    3) Edit the permissions, disable permission inheritance, and then remove all existing permissions.
    4) Add full access to the "ddagentuser" and save your permissions. 
    ```

6. [Provide an executable path](https://docs.datadoghq.com/agent/configuration/secrets-management/?tab=linux#providing-an-executable) to the datadog agent via the main `datadog.yaml` file using the `secret_backend_command` variable:

    ```
    ## datadog.yaml ##

    secret_backend_command: /etc/rapdev-datadog/datadog-secret-backend
    ```

 7. [Provide a configuration](https://github.com/rapdev-io/datadog-secret-backend/blob/main/datadog-secret-backend.yaml.example) for the secrets executable. Documentation for each supported provider can be found [here](https://github.com/rapdev-io/datadog-secret-backend/tree/main/docs).

## Usage

Reference each supported backend type's documentation on specific usage examples and configuration options.

## License

[BSD-3-Clause License](LICENSE)
