# Package Signing Payload

This component populates the Linux Packages signing information in the `inventories` product in DataDog. They fill the `host_signing` table.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 12h (see `inventories_max_interval` in the config). It has a different collection timeframe than the other inventory payloads.

# Format

The payload is a JSON dict with a list of keys, each having the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `agent_version` - **string**: the version of the Agent.
- `signing_keys` - **list of dict of string to JSON type**
  - `fingerprint` - **string**: the 8-char long key fingerprint.
  - `expiration_date` - **string**: the expiration date of the key.
  - `key_type` - **string**: the type of key. which represents how it is referenced in the host. Possible values are "signed-by", "trusted" or "debsig" for debianoids, "repo" or "rpm" for redhat-like (including SUSE)
  

## Example Payload

Here an example of an signing inventory payload:

```
{
    "hostname": "totoro",
    "timestamp": 1631281754507358895,
    "agent_version: "7.50.0",
    "signing_keys": [
      {
        "fingerprint": "12345ABC",
        "expiration_date": "2023-02-12",
        "key_type": "trusted",
      },
      {
        "fingerprint": "DEF90874",
        "expiration_date": "9999-12-24",
        "key_type": "debsig",
      }
    ]
}
```
