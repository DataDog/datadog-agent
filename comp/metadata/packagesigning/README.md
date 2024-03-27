# Package Signing Payload

This component populates the Linux Packages signing information in the `inventories` product in DataDog. They fill the `host_signing` table.

This is enabled by default but can be turned off using `inventories_enabled` or `enable_signing_metadata_collection` config.

The payload is sent every 12h. It has a different collection timeframe than the other inventory payloads.

# Format

The payload is a JSON dict with a list of keys, each having the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `signing_keys` - **list of dict of string to JSON type**
  - `fingerprint` - **string**: the 40-char long key fingerprint, as the 16-char is not supposed to be unique (https://www.rfc-editor.org/rfc/rfc4880#section-3.3)
  - `expiration_date` - **string**: the expiration date of the key.
  - `key_type` - **string**: the type of key. which represents how it is referenced in the host. Possible values are "signed-by", "trusted" or "debsig" for DEB-based distributions, "repo" or "rpm" for RPM-based distributions.
  - `repositories` - **list of dict of string to JSON type**
    - `name` - **string**: a unique repository name signed by the above key according to host configuration. In DEB-based distribution it is information from sources.list files. In RPM-based it is the baseurl or mirrorlist field from repo file.
    - `enabled` - **boolean**: true if the repo is enabled, false otherwise
    - `gpgcheck` - **boolean**: true if GPG signature check on packages is enabled, false otherwise
    - `repo_gpgcheck` - **boolean**: true if GPG signature check on repodata is enabled, false otherwise


## Example Payload

Here an example of an signing inventory payload:

```
{
    "hostname": "totoro",
    "timestamp": 1631281754507358895,
    "signing_metadata": {
      "signing_keys": [
        {
          "fingerprint": "1A2B3C4D5E6F0123456789ABCDEF0987654321FF",
          "expiration_date": "2023-02-12",
          "key_type": "trusted",
        },
        {
          "fingerprint": "F1E2D3C4B5A67890FF1234567899ABCDEF654321",
          "expiration_date": "9999-12-24",
          "key_type": "debsig",
          "repositories": [
            {
              "name": "https://apt.datadoghq.com/ stable 7",
              "enabled": true,
              "gpgcheck": false,
              "repo_gpgcheck": false},
            {"name": "https://yum.datadoghq.com/stable/7/x86_64/",
              "enabled": true,
              "gpgcheck": true,
              "repo_gpgcheck": false},
          ]
        }
      ]
    }
}
```
