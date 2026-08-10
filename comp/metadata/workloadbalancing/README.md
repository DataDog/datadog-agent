# Workload Balancing Metadata Payload

This package populates the Agent workload balancing fields in the `inventories` product in DataDog.

This is enabled by default if `agent_workload_balancing.enabled` is set to true, and can be turned off
using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated
with at most 1 update every minute (see `inventories_min_interval`).

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the Agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `workload_balancing_metadata` - **dict of string to JSON type**:
  - `enabled` - **boolean**: describes if Agent workload balancing has been enabled in the Agent configuration.
  - `groups` - **dict of string to string**: the state this Agent holds for each workload balancing group it
    knows about (`active`, `standby` or `unmanaged`).

## Example Payload

Here an example of an inventory payload:

```
{
    "hostname": "COMP-GQ7WQN6HYC",
    "workload_balancing_metadata": {
        "enabled": true,
        "groups": {
            "group-a": "active",
            "group-b": "standby"
        }
    },
    "timestamp": 1716985696922603000
}
```
