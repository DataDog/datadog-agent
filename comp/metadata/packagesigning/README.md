# Package Signing Payload

This component populates the Linux Packages signing information in the `inventories` product in DataDog. They fill the `host_signing` table.

This is enabled by default on Linux hosts using `apt`, `yum`, `dnf` or `zypper` as package manager, and can be turned
off using the `enable_signing_metadata_collection` config. `enable_metadata_collection` only stops the payload from
being sent: it is still available in the flare (`metadata/inventory/signing.json`) and through the
`/agent/metadata/package-signing` endpoint. `inventories_enabled` has no effect on this payload. It is also disabled
when no install information is available (set through the Agent API `/agent/install-info` endpoint, from the
`DD_INSTALL_INFO_*` environment variables or from the `install_info` file), or when the install tool contains `helm`,
`docker`, `operator` or `kube` (containerized installations). This is evaluated once at startup: install information set
later doesn't enable the payload.

The first payload is sent about 1 minute after startup (`inventories_first_run_delay`), then every 12h. It has a
different collection timeframe than the other inventory payloads. Both the minimum and maximum intervals are 12h, and
the payload is only checked every minimum interval, so the jitter applied to the other inventory payloads never makes it
come early.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `timestamp` - **int**: the timestamp when the payload was created.
- `signing_metadata` - **dict**:
  - `signing_keys` - **list of dict of string to JSON type** (`null` if no key was found). Each subkey of a key is
    reported as its own entry, with its own fingerprint and expiration date and the same repositories.
    - `fingerprint` - **string**: the 40-char long key fingerprint, as the 16-char is not supposed to be unique (https://www.rfc-editor.org/rfc/rfc4880#section-3.3).
      Two special values are used: `nokey` for repositories configured without a key, and `keynotfound` when the key
      file can't be opened or the HTTP request for the key URL fails. If the content is read but isn't a valid key
      (including an HTTP error page), no entry is added for these repositories. Collection errors are only logged at debug
      level, and an error while reading one key or configuration file (ex: a missing `sources.list`, or a debsig policy
      pointing to a missing keyring) can stop the collection of the remaining keys of the same type, so the list can be
      incomplete.
    - `expiration_date` - **string**: the expiration date of the key, as `YYYY-MM-DD`. `9999-12-31` if the key never
      expires (also used for `nokey` and `keynotfound`).
    - `key_type` - **string**: the type of key. which represents how it is referenced in the host. Possible values are "signed-by", "trusted" or "debsig" for DEB-based distributions, "repo" or "rpm" for RPM-based distributions.
    - `repositories` - **list of dict of string to JSON type**: the repositories signed by this key. Always empty for
      "trusted", "debsig" and "rpm" keys (and for "repo" keys used only with `metalink`, see below): `null`, or `[]` when the same key was found in several places.
      - `name` - **string**: a unique repository name signed by the above key according to host configuration. In
        DEB-based distributions it is the sources.list entry after the options (ex: `https://apt.datadoghq.com/ stable 7`).
        Only the one-line sources format is supported: each non-indented, non-comment line of a deb822 `.sources` file (the default on Ubuntu
        24.04 and later) is reported as a separate `nokey` repository.
        In RPM-based distributions there is one entry for each `baseurl` and one for the `mirrorlist` of the repo
        file. `metalink` URLs are not reported, so a "repo" key whose repositories only use `metalink` has
        empty `repositories` (`null`, or `[]` when the same key was found in several places). Variables such as `$releasever` or `$basearch` are not expanded: a local `gpgkey`
        file path using them is reported as `keynotfound`, and a URL using them usually adds no entry (the error page
        isn't a valid key).
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
                        "name": "http://archive.ubuntu.com/ubuntu jammy main restricted",
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
