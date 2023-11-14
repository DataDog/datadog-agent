# Inventory Signing Payload

This component populates the Linux Packages signing information in the `inventories` product in DataDog. They fill the `host_signing` table.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 12h (see `inventories_max_interval` in the config) or whenever it's updated with at most 1
update every 5 minutes (see `inventories_min_interval`).

# Content

The `Set` method from the component allows the rest of the codebase to add any information to the payload.

# Format

The payload is a JSON dict with a list of keys, each having the following fields

- `fingerprint` - **string**: the 8-char long key fingerprint.
- `expiration_date` - **string**: the expiration date of the key.
- `key_type` - **string**: the type of key. which represents how it is referenced in the host. Possible values are "signed-by", "trusted" or "debsig" for debianoids, "repo" or "rpm" for redhat-like (including SUSE)
  

## Example Payload

Here an example of an signing inventory payload:

```
{
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
