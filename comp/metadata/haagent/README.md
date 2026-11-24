# HA Agent Payload

This package populates some of the ha-agent related fields in the `inventories` product in DataDog. More specifically the
`datadog_agent_ha_agent` table.

This is enabled by default and can be turned off using the `inventories_enabled` or `enable_metadata_collection`
config. The payload is sent even when HA
Agent is disabled, in which case `ha_agent_metadata` is `null`. HA Agent is enabled when `ha_agent.enabled` is `true` and
`config_id` is set.

The payload is sent every 10min (see `inventories_max_interval` in the config).

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the ha-agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `ha_agent_metadata` - **dict of string to JSON type** (`null` when HA Agent is disabled):
  - `enabled` - **boolean**: describes if the HA Agent has been enabled in the Agent configuration. Always `true` when
    present.
  - `state` - **string**: HA Agent state: `active`, `standby`, or `unknown` (before the Agent knows its state).

## Example Payload

Here an example of an inventory payload:

```json
{
    "hostname": "my-host",
    "ha_agent_metadata": {
        "enabled": true,
        "state": "active"
    },
    "timestamp": 1716985696922603000
}
```
