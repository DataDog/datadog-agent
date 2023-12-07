# Package Signing Payload

This component populates the Linux Packages signing information in the `inventories` product in DataDog. They fill the `host_signing` table.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 12h (see `inventories_max_interval` in the config). It has a different collection timeframe than the other inventory payloads.

# Format

The payload is a JSON dict with a list of keys, each having the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `signing_keys` - **list of dict of string to JSON type**
  - `fingerprint` - **string**: the 16-char long key fingerprint.
  - `expiration_date` - **string**: the expiration date of the key.
  - `key_type` - **string**: the type of key. which represents how it is referenced in the host. Possible values are "signed-by", "trusted" or "debsig" for DEB-based distributions, "repo" or "rpm" for RPM-based distributions.
  - `repositories` - **list of string to JSON type**
    - `repo_name` - **string**: a unique repository name signed by the above key according to host configuration. In DEB-based distribution it is the aggregation of the repository information from sources.list files. In RPM-based it is the baseurl or mirrorlist field from repo file.
  

## Example Payload

Here an example of an signing inventory payload:

```
{
    "hostname": "totoro",
    "timestamp": 1631281754507358895,
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
        "repositories": [
          {"repo_name": "https://apt.datadoghq.com/stable/7"},
          {"repo_name": "https://yum.datadoghq.com/stable/7/x86_64/"},
        ]
      }
    ]
}
```
