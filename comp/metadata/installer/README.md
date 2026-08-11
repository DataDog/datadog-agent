# Installer metadata Payload

This package populates the fleet installer related fields in the `inventories` product in DataDog.

This is enabled by default but can be turned off using `inventories_enabled` config.

The payload is sent every 10min (see `inventories_max_interval` in the config).

## How it is collected

The installer daemon runs as `root` and the Agent runs as `dd-agent`, so the Agent
cannot read the daemon's local API (`installer.sock`, mode `0700`) — and should not:
every route on it except `/status` installs, removes or promotes packages as root.

Instead the daemon exposes a second, read-only listener — a unix socket at
`/opt/datadog-packages/run/installer-status.sock`, or the `\\.\pipe\DD_INSTALLER_STATUS`
named pipe on Windows — permissioned so the Agent user can read it, following
system-probe (mode `0720` with the socket's group set to the Agent user's; a DACL
granting the `ddagentuser` SID on Windows). This component reads that endpoint.
Access control is the socket permissions; there is no auth token, as with
system-probe.

The values here are therefore only as trustworthy as the local machine: anything
that can bind the socket path or create the named pipe before the daemon does can
answer instead of it. Treat this payload as host-reported inventory, not as an
attested fact.

Note that `comp/updater/daemonchecker` already dials the *local* API to decide
whether the daemon is running. That check cannot succeed from the Agent for the
reason above — it gets `EACCES` on the `0700` socket and swallows it — so its result
is not a usable reachability signal and is not what this component reads. Repointing
`daemonchecker` at this socket, so the Agent has a single reachability primitive, is
a follow-up: it flips the reported state on every Linux host that runs the installer,
which deserves its own change.

A host without a running installer daemon is the normal case, not an error. The
component never fails and never omits the payload: it reports
`installer_reachable: false` and logs at debug level. An explicit `false` is a fact
about the host, whereas silence is indistinguishable from a collection bug.

# Format

The payload is a JSON dict with the following fields

- `hostname` - **string**: the hostname of the agent as shown on the status page.
- `timestamp` - **int**: the timestamp when the payload was created.
- `uuid` - **string**: a unique identifier of the agent, used in case the hostname is empty.
- `installer_metadata` - **dict of string to JSON type**:
  - `installer_reachable` - **bool**: whether the Agent could reach the installer
    daemon's status API. When false, every other field below is absent.
  - `installer_version` - **string**: the version of the running installer daemon.
  - `available_disk_space` - **int**: free space, in bytes, on the partition holding
    the packages directory. Absent when the daemon could not determine it — this is
    deliberately distinct from a reported `0`, which means the disk really is full.
    The daemon also reports this on its Remote Config state
    (`pbgo.ClientUpdater.AvailableDiskSpace`); the duplication is intentional, as this
    payload is meant to become the route for host signals that ride on that state
    today.

## Example Payload

Here an example of an inventory payload:

```
{
    "hostname": "my-host",
    "timestamp": 1631281754507358895,
    "uuid": "706b8f8c-9b9c-4a1f-8e2a-1f3d5c7e9b11",
    "installer_metadata": {
        "installer_reachable": true,
        "installer_version": "7.76.0",
        "available_disk_space": 12884901888
    }
}
```

And on a host with no installer daemon:

```
{
    "hostname": "my-host",
    "timestamp": 1631281754507358895,
    "uuid": "706b8f8c-9b9c-4a1f-8e2a-1f3d5c7e9b11",
    "installer_metadata": {
        "installer_reachable": false
    }
}
```
