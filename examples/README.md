# Datadog Agent Example Configurations

This is a collection of example `datadog-agent.yaml` files to get you started with Datadog. Consult the
[config_template](https://github.com/DataDog/datadog-agent/blob/main/pkg/config/config_template.yaml) for a full list of configuration options. 

To use these add your `api_key` and if necessary update the `site`. Add an `env` 
tag to the `env:` key and any other required tags. If these parameters are set 
with environment variables, they can be commented out. 

Add any other configuration settings needed, then you can copy the file to `/etc/datadog-agent/datadog.yaml` 
for Linux systemsor `%ProgramData%\Datadog\datadog.yaml` for Windows and restart the Datadog Agent.  