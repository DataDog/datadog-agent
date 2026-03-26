# comp/publishermetadatacache

**Team:** windows-products
**Platform:** Windows only (`//go:build windows`)
**Package:** `github.com/DataDog/datadog-agent/comp/publishermetadatacache`

## Purpose

The `publishermetadatacache` component caches Windows Event Log publisher metadata handles obtained via `EvtOpenPublisherMetadata`. Opening a publisher metadata handle is an expensive Windows API call; without a cache, every event message format request would re-open the handle for the same publisher. The component keeps those handles alive for the lifetime of the agent run and provides a single `FormatMessage` entry point used by Windows Event Log tailers and checks.

## Key Elements

### Component interface

`comp/publishermetadatacache/def/component.go`

```go
type Component interface {
    FormatMessage(publisherName string, event evtapi.EventRecordHandle, flags uint) (string, error)
    Flush()
}
```

| Method | Description |
|---|---|
| `FormatMessage(publisher, event, flags)` | Format the message string for `event` using the cached handle for `publisher`. Opens and caches a new handle on the first call for a given publisher name. |
| `Flush()` | Release all cached handles. Called automatically on component shutdown via the lifecycle `OnStop` hook. |

### Implementation

The component is a thin wrapper around `pkg/util/winutil/eventlog/publishermetadatacache` (`publishermetadatacachepkg`), which holds the actual handle map. The `impl` package (`comp/publishermetadatacache/impl/publishermetadatacache.go`) wires the package-level cache into the fx component lifecycle:

```go
func NewComponent(reqs Requires) Provides {
    cache := publishermetadatacachepkg.New(winevtapi.New())
    reqs.Lifecycle.Append(compdef.Hook{
        OnStop: func(_ context.Context) error {
            cache.Flush()
            return nil
        },
    })
    return Provides{Comp: cache}
}
```

`Requires` only needs a `compdef.Lifecycle`; there are no other dependencies.

### Handle lifecycle

Handles are opened lazily on the first `FormatMessage` call for a given publisher name and remain in the cache until `Flush()` is called at shutdown. There is no TTL or eviction; the cache grows monotonically over the agent's lifetime (bounded by the number of distinct publishers on the host).

## Usage

The component is wired into the Windows agent binary in `cmd/agent/subcommands/run/command_windows.go`.

The primary consumers are:

- **Windows Event Log tailer** (`pkg/logs/tailers/windowsevent/tailer.go`): receives the component via constructor injection and calls `FormatMessage` for every event to render human-readable log messages.
- **Windows Event Log check** (`comp/checks/windowseventlog/impl/check/`): uses the component for message formatting in the check submitter and message filter.

Example from the tailer:

```go
func NewTailer(
    evtapi evtapi.API,
    source *sources.LogSource,
    config *Config,
    outputChan chan *message.Message,
    registry auditor.Registry,
    publisherMetadataCache publishermetadatacache.Component,
) *Tailer
```
