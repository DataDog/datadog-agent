# Inventory Host Payload

This package populates some of the Agent-related fields in the `inventories` product in DataDog. More specifically the
`host_gpu_agent` table.

This is enabled by default but can be turned off using `inventories_gpu_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every 5 minutes (see `inventories_min_interval`).

# Content

The `Set` method from the component allows the rest of the codebase to add any information to the payload.

# Format

The payload is a JSON dict with the following fields

// TODO!

## Example Payload

Here an example of an inventory payload:

```
//TODO!
```
