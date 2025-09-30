# Inventory Checks Payload

This package populates some of the checks-related fields in the `inventories` product in DataDog.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every 1 minute (see `inventories_min_interval`).

# Content

The `Set` method from the component allow the rest of the codebase to add any Metadata to a check running in the collector.
Metadata can include the check version, the version of the monitored software, ... It depends on each check.

For every running check, no matter if it registered extra metadata or not, we send: name, ID, configuration,
configuration provider. Sending checks configuration can be disabled using `inventories_checks_configuration_enabled`.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `check_metadata` - **dict of string to list**: dictionary with check names as keys; values are a list of the metadata for each
  instance of that check.
  Each instance is composed of:
    - `config.hash` - **string**: the instance ID for this instance (as shown in the status page).
    - `config.provider` - **string**: where the configuration came from for this instance (disk, docker labels, ...).
    - `config.source` - **string**: the file path if it exists.
    - `init_config` - **string**: the `init_config` part of the configuration for this check instance.
    - `instance_config` - **string**: the YAML configuration for this check instance
    - Any other metadata registered by the instance (instance version, version of the software monitored, ...).
- `logs_metadata` - **dict of string to list**: dictionary with the log source names as keys; values are a list of the metadata
  for each instance of that log source.
  Each instance is composed of:
    - `config` - **string**: the canonical JSON of the log source configuration.
    - `state` - **dict of string**: the current state of the log source.
      - `status` - **string**: one of `pending`, `error` or `success`.
      - `error` - **string**: the error description if any.
    - `integration_name` - **string**: the name of the integration, can be empty.
    - `service` - **string**: the service name of the log source.
    - `source` - **string**: the log source name.
    - `tags` - **list of string**: a list of tags attached to the log source.

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

## Example Payload

Here an example of an inventory payload:

```
{
    "check_metadata": {
        "cpu": [
            {
                "config.hash": "cpu",
                "config.provider": "file",
                "init_config": "",
                "instance_config: {}
            }
        ],
        "disk": [
            {
                "config.hash": "disk:e5dffb8bef24336f",
                "config.provider": "file",
                "init_config": "",
                "instance_config: {}
            }
        ],
        "file_handle": [
            {
                "config.hash": "file_handle",
                "config.provider": "file",
                "init_config": "",
                "instance_config: {}
            }
        ],
        "io": [
            {
                "config.hash": "io",
                "config.provider": "file",
                "init_config": "",
                "instance_config: {}
            }
        ],
        "load": [
            {
                "config.hash": "load",
                "config.provider": "file",
                "init_config": "",
                "instance_config: {}
            }
        ],
        "redisdb": [
            {
                "config.hash": "redisdb:6e5e79e5b724c83a",
                "config.provider": "container",
                "init_config": "test: 21",
                "instance_config": "host: localhost\nport: 6379\ntags:\n- docker_image:redis\n- image_name:redis\n- short_image:redis",
                "version.major": "7",
                "version.minor": "0",
                "version.patch": "2",
                "version.raw": "7.0.2",
                "version.scheme": "semver"
            },
            {
                "config.hash": "redisdb:c776ecdbdded09b8",
                "config.provider": "container",
                "init_config": "test: 21",
                "instance_config": "host: localhost\nport: 7379\ntags:\n- docker_image:redis\n- image_name:redis\n- short_image:redis",
                "version.major": "7",
                "version.minor": "0",
                "version.patch": "2",
                "version.raw": "7.0.2",
                "version.scheme": "semver"
            }
        ]
    },
    "logs_metadata": {
        "redisdb": [
            {
                "config": "{\"path\":\"/var/log/redis_6379.log\",\"service\":\"myredis2\",\"source\":\"redis\",\"type\":\"file\",\"tags\":[\"env:prod\"]}",
                "integration_name": "redis",
                "service": "awesome_cache",
                "source": "source1",
                "state": {
                    "error": "Error: cannot read file /var/log/redis_6379.log: stat /var/log/redis_6379.log: no such file or directory",
                    "status": "error"
                },
                "tags": ["env:prod"]
            }
        ],
        "nginx": [
            {
                "config": "{\"path\":\"/var/log/nginx/access.log\",\"service\":\"nginx\",\"source\":\"nginx\",\"type\":\"file\"}",
                "integration_name": "nginx",
                "service": "nginx",
                "source": "source2",
                "state": {
                    "error": "",
                    "status": "success"
                },
                "tags": []
            }
        ]
    }
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
