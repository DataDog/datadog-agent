# Inventory Payload

This package populates some of the agent-related fields in the `inventories` product in DataDog.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every 5 minutes (see `inventories_min_interval`).

# Content

The package offers 2 method to add data to the payload: `SetAgentMetadata` and `SetCheckMetadata`. As the name suggests,
checks use `SetCheckMetadata` and each metadata is linked to a check ID. Everything agent-related uses the other one.

Any part of the agent can add metadata to the inventory payload.

## Check metadata

`SetCheckMetadata` registers data per check instance. Metadata can include the check version, the version of the
monitored software, ... It depends on each check.

## Agent metadata

`SetAgentMetadata` registers data about the agent itself.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `check_metadata` - **dict of string to list**: dictionary with check names as keys; values are a list of the metadata for each
  instance of that check.
  Each instance is composed of:
    - `last_updated` - **int**: timestamp of the last metadata update for this instance
    - `config.hash` - **string**: the instance ID for this instance (as shown in the status page).
    - `config.provider` - **string**: where the configuration came from for this instance (disk, docker labels, ...).
    - Any other metadata registered by the instance (instance version, version of the software monitored, ...).
<!-- NOTE: when modifying this list, please also update the constants in `inventories.go` -->
- `agent_metadata` - **dict of string to JSON type**:
  - `cloud_provider` - **string**: the name of the cloud provider detected by the Agent (omitted if no cloud is detected).
  - `hostname_source` - **string**: the source for the agent hostname (see pkg/util/hostname.go:GetHostnameData).
  - `version` - **string**: the version of the Agent.
  - `flavor` - **string**: the flavor of the Agent. The Agent can be build under different flavor such as standalone
    dogstatsd, iot, serverless ... (see `pkg/util/flavor` package).
  - `install_method_tool` - **string**: the name of the tool used to install the agent (ie, Chef, Ansible, ...).
  - `install_method_tool_version` - **string**: the tool version used to install the agent (ie: Chef version, Ansible
    version, ...). This defaults to `"undefined"` when not installed through a tool (like when installed with apt, source
    build, ...).
  - `install_method_installer_version` - **string**:  The version of Datadog module (ex: the Chef Datadog package, the Datadog Ansible playbook, ...).

## Example Payload

Here an example of an inventory payload:

```
{
    "agent_metadata": {
        "hostname_source": "os"
    },
    "check_metadata": {
        "cpu": [
            {
                "config.hash": "cpu",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "disk": [
            {
                "config.hash": "disk:e5dffb8bef24336f",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "file_handle": [
            {
                "config.hash": "file_handle",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "io": [
            {
                "config.hash": "io",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "load": [
            {
                "config.hash": "load",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "memory": [
            {
                "config.hash": "memory",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "network": [
            {
                "config.hash": "network:d884b5186b651429",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "ntp": [
            {
                "config.hash": "ntp:d884b5186b651429",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ],
        "uptime": [
            {
                "config.hash": "uptime",
                "config.provider": "file",
                "last_updated": 1631281744506400319
            }
        ]
    },
    "hostname": "my-host",
    "timestamp": 1631281754507358895
}
```
