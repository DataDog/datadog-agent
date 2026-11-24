# Package Signing Payload

This component populates the Linux Packages signing information in the `inventories` product in DataDog. They fill the `host_signing` table.

This is enabled by default on Linux hosts using `apt`, `yum`, `dnf` or `zypper` as package manager, and can be turned off
using the `enable_signing_metadata_collection` or `enable_metadata_collection` config. `inventories_enabled` has no
effect on this payload. It is also disabled when the `install_info` file can't be read, or when the install method
contains `helm`, `docker`, `operator` or `kube` (containerized installations).

The payload is sent every 12h, and a scheduled payload can come up to 20% early. It has a different collection timeframe
than the other inventory payloads.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `signing_metadata` - **dict**:
  - `signing_keys` - **list of dict of string to JSON type** (`null` if no key was found)
    - `fingerprint` - **string**: the 40-char long key fingerprint, as the 16-char is not supposed to be unique (https://www.rfc-editor.org/rfc/rfc4880#section-3.3).
      Two special values are used: `nokey` for repositories configured without a key, and `keynotfound` when the key
      file or URL can't be read.
    - `expiration_date` - **string**: the expiration date of the key, as `YYYY-MM-DD`. `9999-12-31` if the key never
      expires (also used for `nokey` and `keynotfound`).
    - `key_type` - **string**: the type of key. which represents how it is referenced in the host. Possible values are "signed-by", "trusted" or "debsig" for DEB-based distributions, "repo" or "rpm" for RPM-based distributions.
    - `repositories` - **list of dict of string to JSON type**: the repositories signed by this key. Always `null` for
      "trusted", "debsig" and "rpm" keys.
      - `name` - **string**: a unique repository name signed by the above key according to host configuration. In
        DEB-based distributions it is the sources.list entry after the options (ex: `https://apt.datadoghq.com/ stable 7`).
        In RPM-based distributions it is each `baseurl`, or the `mirrorlist` field, from the repo file.
      - `enabled` - **boolean**: true if the repo is enabled, false otherwise. Always true for DEB-based distributions.
      - `gpgcheck` - **boolean**: true if GPG signature check on packages is enabled, false otherwise
      - `repo_gpgcheck` - **boolean**: true if GPG signature check on repodata is enabled, false otherwise


## Example Payload

Here an example of an signing inventory payload from a DEB-based host:

```
{
    "hostname": "my-host",
    "uuid": "00000000-0000-0000-0000-000000000001",
    "timestamp": 1631281754507358895,
    "signing_metadata": {
        "signing_keys": [
            {
                "fingerprint": "1A2B3C4D5E6F0123456789ABCDEF0987654321FF",
                "expiration_date": "2027-02-12",
                "key_type": "trusted",
                "repositories": null
            },
            {
                "fingerprint": "F1E2D3C4B5A67890FF1234567899ABCDEF654321",
                "expiration_date": "9999-12-31",
                "key_type": "signed-by",
                "repositories": [
                    {
                        "name": "https://apt.datadoghq.com/ stable 7",
                        "enabled": true,
                        "gpgcheck": false,
                        "repo_gpgcheck": true
                    }
                ]
            },
            {
                "fingerprint": "nokey",
                "expiration_date": "9999-12-31",
                "key_type": "signed-by",
                "repositories": [
                    {
                        "name": "http://archive.ubuntu.com/ubuntu noble main restricted",
                        "enabled": true,
                        "gpgcheck": false,
                        "repo_gpgcheck": true
                    }
                ]
            }
        ]
    }
}
```
