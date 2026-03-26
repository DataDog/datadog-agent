# pkg/util/subscriptions

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/subscriptions`

## Purpose

`subscriptions` provides a type-safe, Fx-integrated pub/sub bus for low-frequency
inter-component events. A _transmitting_ component calls `Notify()` to broadcast a
message; one or more _receiving_ components read from a buffered channel.

Matching is done by message type: the Go generic parameter `M` on both
`Transmitter[M]` and `Receiver[M]` must be the same concrete type. Because the
message type is the routing key, **it must be unique across the codebase** — do
not use a primitive type like `string` or `int`.

> **Warning:** This package is not designed for high-throughput messaging (e.g.
> metric samples). It should be used only for events that happen on a per-second
> scale or slower.

## Key elements

### Types

| Type | Description |
|------|-------------|
| `Message` | Marker interface (`interface{}`). All message types implement it implicitly. |
| `Transmitter[M Message]` | Sends messages to all subscribed receivers. Contains an `fx.In`-tagged slice of channels. |
| `Receiver[M Message]` | Holds the channel a component reads from. Contains an `fx.Out`-tagged channel that Fx injects into the transmitter's slice. |

### Transmitter

```go
type Transmitter[M Message] struct {
    fx.In
    Chs []chan M `group:"subscriptions"`
}

func (tx Transmitter[M]) Notify(message M)
```

`Notify` sends `message` to every associated receiver channel. It **blocks**
if any receiver's channel is full, and **panics** if a channel is closed. Keep
receivers drained.

### Receiver

```go
type Receiver[M Message] struct {
    fx.Out
    Ch chan M `group:"subscriptions"`
}

func NewReceiver[M Message]() Receiver[M]
```

`NewReceiver` creates a receiver with a channel buffered to size 1. The
`Receiver` struct carries `fx.Out` so Fx automatically appends `Ch` to the
transmitter's `Chs` slice.

A zero-valued `Receiver[M]{}` is valid and safe to return from an Fx
constructor when a component does not wish to subscribe (e.g., when disabled by
configuration). The `nil` channel it carries is skipped by `Notify`.

## Usage

### Defining a message type

Define a dedicated named type in the transmitting component's package:

```go
// package announcer
type Announcement struct {
    Message string
}
```

### Transmitting component

Declare `subscriptions.Transmitter[announcer.Announcement]` as an Fx
dependency. Fx will populate `Chs` with the channels of all registered
receivers at startup:

```go
func newAnnouncer(tx subscriptions.Transmitter[Announcement]) Component {
    return &announcer{tx: tx}
}

func (a *announcer) announce(msg Announcement) {
    a.tx.Notify(msg)
}
```

### Receiving component

Return a `subscriptions.Receiver[announcer.Announcement]` as an Fx output value
(alongside the component itself). Fx registers `Ch` with the transmitter's
group automatically:

```go
func newListener() (Component, subscriptions.Receiver[announcer.Announcement]) {
    rx := subscriptions.NewReceiver[announcer.Announcement]()
    return &listener{rx: rx}, rx
}

func (l *listener) run() {
    for {
        select {
        case msg := <-l.rx.Ch:
            // handle msg
        case <-ctx.Done():
            return
        }
    }
}
```

### Opting out at runtime

If the component decides it does not need the subscription (e.g., a feature is
disabled), return a zero-valued receiver so that `Notify` skips it:

```go
func newListener(cfg config.Component) (Component, subscriptions.Receiver[announcer.Announcement]) {
    if !cfg.GetBool("feature.enabled") {
        return &listener{}, subscriptions.Receiver[announcer.Announcement]{}
    }
    rx := subscriptions.NewReceiver[announcer.Announcement]()
    return &listener{rx: rx}, rx
}
```

### Fx wiring note

Because `Receiver` carries `fx.Out`, any component that provides a non-nil
receiver will be instantiated by Fx even if nothing else in the dependency
graph depends on it. This is usually the desired behaviour — the subscriber
should start when the transmitter starts. The opt-out pattern above is the
escape hatch when that is not wanted.

## Relationship to other packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/util/fxutil` | [fxutil.md](fxutil.md) | `subscriptions` integrates with fxutil's fx-based component model. `Transmitter[M]` and `Receiver[M]` use `fx.In` / `fx.Out` and the `group:"subscriptions"` value-group tag that Fx resolves at startup. Components that opt out return a zero-valued `Receiver` so that `fxutil.GetAndFilterGroup` can safely skip nil channels. |
| `pkg/logs/sources` | [../logs/sources.md](../logs/sources.md) | `pkg/logs/sources.LogSources` implements its own pub/sub pattern (scheduler → launcher) independently of `pkg/util/subscriptions`. The key difference: `LogSources` is not Fx-wired — subscribers call `SubscribeForType`/`SubscribeAll` explicitly at runtime, and channels are unbuffered. `pkg/util/subscriptions` is Fx-wired (channels are registered at app construction) and uses a buffer of 1. Use `pkg/util/subscriptions` for inter-*component* events in the Fx graph; use `LogSources` for logs-pipeline scheduler-to-launcher wiring. |
| `comp/core/workloadmeta` | [../../comp/core/workloadmeta.md](../../comp/core/workloadmeta.md) | `workloadmeta` exposes its own subscription API (`Subscribe`/`Unsubscribe`) that returns a `chan EventBundle`. This is purpose-built for workload lifecycle events at potentially higher frequency. `pkg/util/subscriptions` is a simpler, more general Fx-integrated bus for low-frequency inter-component notifications. |
