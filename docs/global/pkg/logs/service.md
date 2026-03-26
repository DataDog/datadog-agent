# pkg/logs/service

## Purpose

Provides a lightweight abstraction for a _service_ — a running process or container whose logs the agent is tailing — and a fan-out registry that notifies subscribers when services appear or disappear. This is the mechanism by which log launchers discover and react to dynamic workloads (e.g., new Docker containers, Kubernetes pods).

## Key Elements

| Symbol | Description |
|---|---|
| `Service` | Represents a single observable entity. Fields: `Type` (provider type, e.g. `"docker"`, `"kubernetes"`) and `Identifier` (unique ID within that type). `GetEntityID()` returns a URI-style string `"<type>://<identifier>"`. |
| `NewService(providerType, identifier)` | Constructor for `Service`. |
| `Services` | Thread-safe fan-out registry. Maintains the current list of active services and a set of subscriber channels, keyed by service type or a wildcard. |
| `NewServices()` | Constructor for `Services`. |
| `(s *Services) AddService(service)` | Adds a service to the registry and pushes it to all matching subscriber channels. |
| `(s *Services) RemoveService(service)` | Removes a service from the registry and notifies subscribers. |
| `(s *Services) GetAddedServicesForType(serviceType)` | Returns a `chan *Service` that receives newly added services of the given type. Already-registered services are replayed asynchronously from a goroutine after registration. |
| `(s *Services) GetRemovedServicesForType(serviceType)` | Returns a `chan *Service` that receives removed services of the given type. |
| `(s *Services) GetAllAddedServices()` | Returns a `chan *Service` for added services of any type. Existing services are replayed asynchronously. |
| `(s *Services) GetAllRemovedServices()` | Returns a `chan *Service` for removed services of any type. |

**Replay semantics:** `GetAddedServicesForType` and `GetAllAddedServices` replay existing services from a goroutine (not inline) so callers must not hold locks while consuming from the channel. `GetRemovedServicesForType` / `GetAllRemovedServices` do _not_ replay history.

## Usage

`pkg/logs/service` is consumed by schedulers and launchers:

- **`pkg/logs/schedulers/ad/scheduler.go`** — the Autodiscovery scheduler calls `Services.AddService` / `RemoveService` when containers start and stop, and passes the `Services` object to launchers.
- **`pkg/logs/schedulers/schedulers.go`** and related files — construct `NewServices()` and wire it to multiple log launchers.
- **`comp/logs/agent/agentimpl/agent.go`** — instantiates `Services` at agent startup and passes it down to the pipeline.

Typical pattern for a launcher consuming service events:

```go
added := services.GetAddedServicesForType("docker")
removed := services.GetRemovedServicesForType("docker")

for {
    select {
    case svc := <-added:
        // start tailing logs for svc.Identifier
    case svc := <-removed:
        // stop tailing logs for svc.Identifier
    }
}
```
