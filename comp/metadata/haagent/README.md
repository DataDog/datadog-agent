# Inventory Agent Payload

This package populates some of the ha-agent related fields in the `inventories` product in DataDog. More specifically the
`datadog_agent_ha_agent` table.

This is enabled by default if `ha_agent.enabled` is set to true, and can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every minute (see `inventories_min_interval`).

# Content

## Agent Configuration

Sending Agent configuration can be disabled using `inventories_configuration_enabled`.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the ha-agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `ha_agent_metadata` - **dict of string to JSON type**:
  - `enabled` - **boolean**: describes if the HA Agent has been enabled in the Agent configuration.
  - `state` - **string**: HA Agent state (active or standby).

("scrubbed" indicates that secrets are removed from the field value just as they are in logs)

As the environment configuration and override only affect scalar values (as opposed to slices & maps), combining configurations should be straight-forward to do. Environment variables take precendence over the provided configuration, and runtime overrides take precendence over that.

## Example Payload

Here an example of an inventory payload:

```
{
    "hostname": "COMP-GQ7WQN6HYC",
    "ha_agent_metadata": {
        "enabled": true,
        "state": "active"
    },
    "timestamp": 1716985696922603000
}
```
