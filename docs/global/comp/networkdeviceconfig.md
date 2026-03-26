# comp/networkdeviceconfig

**Team:** ndm-integrations

## Purpose

`comp/networkdeviceconfig` (Network Config Management, NCM) enables the agent
to retrieve running and startup configurations from network devices over SSH.
It is used by the Network Device Monitoring (NDM) subsystem to push device
configuration data to the Datadog backend.

The component establishes an SSH session to a device identified by IP address,
runs the appropriate CLI command (`show running-config` or
`show startup-config`), and returns the raw output as a string.

## Key elements

### Component interface

```go
// comp/networkdeviceconfig/def/component.go
type Component interface {
    RetrieveRunningConfig(ipAddress string) (string, error)
    RetrieveStartupConfig(ipAddress string) (string, error)
}
```

Both methods take a device IP address and return the raw configuration string
or an error. They look up authentication credentials from the pre-configured
device map; if the IP is not found, an error is returned immediately.

### Implementation (`comp/networkdeviceconfig/impl`)

**`NewComponent(reqs Requires) (Provides, error)`** — reads the NCM config
block from `datadog.yaml`, builds an IP-keyed device map, and returns the
implementation. No lifecycle hooks are registered — the component is stateless
between calls.

**`retrieveConfiguration(ipAddress, commands)`** — internal helper that looks
up credentials, calls `clientFactory.Connect`, iterates over the command list,
and joins outputs with a newline separator. Sessions are opened and closed per
command.

**`RemoteClientFactory` / `SSHClientFactory`** — the SSH connection is
abstracted behind a `RemoteClientFactory` interface (in `remote.go`) to allow
test doubles. The production factory calls `connectToHost`, which dials via
`golang.org/x/crypto/ssh`.

> Note: The current implementation uses `ssh.InsecureIgnoreHostKey()`. SSH key
> authentication is not yet implemented (see the TODO in `config.go`).

### Configuration types

```go
type AuthCredentials struct {
    Username string
    Password string
    Port     string
    Protocol string
}

type DeviceConfig struct {
    IPAddress string
    Auth      AuthCredentials
}
```

Read from the agent config key `network_device_config_management`:

```yaml
network_device_config_management:
  namespace: default
  devices:
    - ip_address: "10.0.0.1"
      auth:
        username: admin
        password: secret
        port: "22"
        protocol: tcp
```

The raw list is converted to a `map[string]DeviceConfig` keyed by IP address
for O(1) lookup at call time.

### fx wiring

```
comp/networkdeviceconfig/fx/fx.go  →  NewComponent (impl)
comp/networkdeviceconfig/mock/mock.go  →  test stub
```

Depends on: `config.Component`, `log.Component`, `compdef.Lifecycle`.

## Usage

The component is registered in the main agent and cluster-agent fx graphs. Any
component that needs to pull device configs declares a dependency on
`networkdeviceconfig.Component` and calls `RetrieveRunningConfig` or
`RetrieveStartupConfig` by device IP.

Typical call pattern:

```go
cfg, err := ndcComp.RetrieveRunningConfig("10.0.0.1")
if err != nil {
    // device not configured or SSH failure
}
// cfg is the raw output of "show running-config"
```

To add support for a new authentication method (e.g., SSH keys or enable
passwords), extend `AuthCredentials` in `comp/networkdeviceconfig/impl/config.go`
and update `connectToHost` in `networkdeviceconfig.go`.

## Related components

| Component / Package | Relationship |
|---|---|
| [`pkg/networkdevice`](../pkg/networkdevice/networkdevice.md) | Provides the shared NDM metadata types and the `integrations.NetworkConfigurationManagement` constant used to tag payloads sent to the Datadog backend. `comp/networkdeviceconfig` retrieves raw device configuration strings; higher-level NDM integrations (e.g. the SNMP check or `pkg/networkconfigmanagement/`) rely on `pkg/networkdevice/metadata` to wrap those strings in `NetworkDevicesMetadata` payloads before forwarding them with event type `EventTypeNetworkConfigManagement`. |
| [`comp/remote-config/rcclient`](remote-config/rcclient.md) | RC-triggered device scans (task type `types.TaskDeviceScan`) can prompt NDM integrations to refresh configuration data. `comp/networkdeviceconfig` does not subscribe to RC directly, but the NDM scan pipeline that calls `RetrieveRunningConfig` / `RetrieveStartupConfig` is typically invoked in response to an `AGENT_TASK` dispatched by `rcclient`. |
