# datadog-secret-backend
> Implementation of Datadog's secret backend command supporting multiple backends


## Obtaining the executable required

**Note**: For sake of ease, the commands following will upload the required file(s) to our recommended path(s). You can change those if desired but keep in mind that all/most variables will need to be changed accordingly.

1) Make a new folder in `/etc/` to hold all the files required for this module in one place:

    mkdir -p /etc/rapdev-datadog

2) Download the most recent version of the secret backend module by hitting the latest release endpoint from the `rapdev-io` repo by running the command below:

    ```
    curl -L https://github.com/rapdev-io/datadog-secret-backend/releases/latest/download/datadog-secret-backend-linux-amd64.tar.gz \ 
    -o /tmp/datadog-secret-backend-linux-amd64.tar.gz
    ```

3) Once you have the file from the github repo, you'll need to unzip it to get the actual executable:

    ```
    tar -xvzf /tmp/datadog-secret-backend-linux-amd64.tar.gz \
    -C /etc/rapdev-datadog
    ```

4) (Optional) Remove the old tar'd file:

    ```
    rm /tmp/datadog-secret-backend-linux-amd64.tar.gz
    ```

## Configuring the secrets module

1) Create a new file `datadog-secret-backend.yaml` for holding all the configurations for the secrets module. Store this file in the same location that you did the executable in the previous step (e.g. `/etc/rapdev-datadog/`) An example of the contents is provided below for each of the currently supported secret types:

    ```
    backends:
      datadog-secrets-yaml-file:
        backend_type: 'file.yaml'
        file_path: '/etc/rapdev-datadog/secrets/secrets.yaml'

      datadog-secrets-json-file:
        backend_type: 'file.json'
        file_path: '/etc/rapdev-datadog/secrets/secrets.json'
      
      datadog-secrets-secretsmanager:
        backend_type: 'aws.secrets'
        secret_id: 'arn:aws:secretsmanager:us-east-1:<ACCOUNT_ID>:secret:/<SECRET_NAME>'
        aws_region: 'us-east-1'
      
      datadog-secrets-ssm:
        backend_type: 'aws.ssm'
        secret_id: 'arn:aws:ssm:us-east-1:<ACCOUNT_ID>:secret:/<SECRET_NAME>'
        aws_region: 'us-east-1'
    ```

## Configuring your secrets

- <b>Local File</b>: If you are storing your secrets in a file (JSON or YAML), create the file in the appropriate format with key value pairs mapping to your secret values:

    ```
    ## YAML
    dd_api_key: <MY_API_KEY>
    dd_app_key: <MY_APP_KEY>
    my_secret: <SECRET_VALUE>

    ## JSON
    {
      "dd_api_key": "<MY_API_KEY>",
      "dd_app_key": "<MY_APP_KEY>",
      "my_secret": "<SECRET_VALUE>"
    }
    ```

- <b>AWS SSM</b>:

- <b>AWS SecretsManager</b>:

## Configuring the Agent(s) to use the secrets module


## Accessing your secret values