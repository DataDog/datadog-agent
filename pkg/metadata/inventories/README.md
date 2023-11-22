# Inventory Payload

This package populates some of the agent-related fields in the `inventories` product in DataDog.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every 1 minute (see `inventories_min_interval`).

# Content

The current payload contains 1 section: `check_metadata`.

If the final serialized payload is too big, each section is sent in a different payload with `hostname` and `timestamp`
always present. This is why some fields are duplicated between section, like `agent_version`.

The package offers a method to the Agent codebase to add data to the payload about checks: `SetCheckMetadata`.

## Check metadata

`SetCheckMetadata` registers data per check instance. Metadata can include the check version, the version of the
monitored software, ... It depends on each check.

For every running check, no matter if it registered extra metadata or not, we send: name, ID, configuration,
configuration provider. Sending checks configuration can be disabled using `inventories_checks_configuration_enabled`.


# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `check_metadata` - **dict of string to list**: dictionary with check names as keys; values are a list of the metadata for each
  instance of that check.
  Each instance is composed of:
    - `config.hash` - **string**: the instance ID for this instance (as shown in the status page).
    - `config.provider` - **string**: where the configuration came from for this instance (disk, docker labels, ...).
    - `init_config` - **string**: the `init_config` part of the configuration for this check instance.
    - `instance_config` - **string**: the YAML configuration for this check instance
    - Any other metadata registered by the instance (instance version, version of the software monitored, ...).

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
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
