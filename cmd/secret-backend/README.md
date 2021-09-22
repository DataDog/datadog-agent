# datadog-secret-backend


> **Note**: For the purpose of this installation, the commands following will upload the required file(s) to our recommended path(s). You can change those if desired but keep in mind that all/most variables will need to be changed accordingly.

## Obtaining the executable required

1) Make a new folder in `/etc/` to hold all the files required for this module in one place:

    ```
    ## Linux
    mkdir -p /etc/rapdev-datadog

    ## Windows
    mkdir 'C:\Program Files\rapdev-datadog\'
    ```

2) Download the most recent version of the secret backend module by hitting the latest release endpoint from the `rapdev-io` repo by running one of the commands below:

    ```
    ## Linux (amd64)
    curl -L https://github.com/rapdev-io/datadog-secret-backend/releases/latest/download/datadog-secret-backend-linux-amd64.tar.gz \ 
    -o /tmp/datadog-secret-backend-linux-amd64.tar.gz

    ## Linux (386)
    curl -L https://github.com/rapdev-io/datadog-secret-backend/releases/latest/download/datadog-secret-backend-linux-386.tar.gz \ 
    -o /tmp/datadog-secret-backend-linux-386.tar.gz

    ## Windows (amd64)
    Invoke-WebRequest https://github.com/rapdev-io/datadog-secret-backend/releases/latest/download/datadog-secret-backend-windows-amd64.zip ^
    -OutFile 'C:\Program Files\rapdev-datadog\' 

    ## Windows (386)
    Invoke-WebRequest https://github.com/rapdev-io/datadog-secret-backend/releases/latest/download/datadog-secret-backend-windows-386.zip ^ 
    -OutFile 'C:\Program Files\rapdev-datadog\'
    ```

3) Once you have the file from the github repo, you'll need to unzip it to get the executable:

    ```
    ## Linux (amd64, change end of filename to "386" if needed)
    tar -xvzf /tmp/datadog-secret-backend-linux-amd64.tar.gz \
    -C /etc/rapdev-datadog

    ## Windows (amd64, change end of filename to "386" if needed)
    tar -xvzf 'C:\Program Files\rapdev-datadog\datadog-secret-backend-windows-amd64.tar.gz' ^
    -C 'C:\Program Files\rapdev-datadog\'
    ```

4) (Optional) Remove the old tar'd file:

    ```
    ## Linux
    rm /tmp/datadog-secret-backend-linux-amd64.tar.gz

    ## Windows
    del /f 'C:\Program Files\rapdev-datadog\datadog-secret-backend-windows-amd64.tar.gz'
    ```

5) Update the executable to have the required permissions. Datadog agent expects the executable to only
be used by the `dd-agent` user for Linux and `ddagentuser` for Windows.

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

- <b>Local File</b>: If you are storing your secrets in a file (JSON or YAML), create the file in the appropriate format with key value pairs mapping to your secret values. The path and name to this file should be passed in via the `file_path` in the example above accordingly:

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

In the `datadog.yaml`, there is a flag called `secret_backend_command`. Additionally, if you used a different name for the secrets module configuration file (e.g. `datadog-secret-backend.yaml`), you can provide the name for the file via the config argument Please provide the path to your executable for that file:

    ```
    ## Linux
    secret_backend_command: /etc/rapdev-datadog/datadog-secret-backend
    
    ### Only provide this flag if you changed the name of the configuration file
    secret_backend_arguments:
      - '-config'
      - '/etc/rapdev-datadog/secret-backends.yaml'

    ## Windows
    secret_backend_command: 'C:\Program Files\rapdev-datadog\datadog-secret-backend.exe'
    
    ### Only provide this flag if you changed the name of the configuration file
    secret_backend_arguments:
      - '-config'
      - 'C:\Program Files\rapdev-datadog\secret-backends.yaml'
    ```


## Accessing your secret values

To access your secret values, you should use the `"ENC[]"` notation along with the name provided for the main header of your configuration file (e.g. `datadog-secret-backend.yaml` or whatever other name was provided) along with the name of the secret in your secrets file, AWS SSM, or AWS SecretsManager. For example, to access the `dd_api_key` secret from the `datadog-secrets-yaml-file` backend's section, you would pass in the following value:

    ```
    "ENC[datadog-secrets-yaml-file:dd_api_key]"
    ```

