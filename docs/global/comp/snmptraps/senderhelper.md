# comp/snmptraps/senderhelper

## Purpose

`senderhelper` is a **test-only** helper package (compiled only under the `test` build tag). It provides a pre-wired set of fx options that stand up a mock `Sender` and inject it as the default sender of a mock demultiplexer. This removes boilerplate from SNMP traps unit tests that need to assert on emitted metrics or Event Platform events.

The package exists because `sender.Sender` is not yet exposed as a standalone fx component. Until it is, each test that needs a controllable sender would otherwise have to replicate the same wiring. `senderhelper.Opts` centralises that wiring.

## Key elements

### `Opts`

```go
var Opts = fx.Options(
    defaultforwarder.MockModule(),
    demultiplexerimpl.MockModule(),
    hostnameimpl.MockModule(),
    fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
    fx.Provide(func() (*mocksender.MockSender, sender.Sender) {
        mockSender := mocksender.NewMockSender("mock-sender")
        mockSender.SetupAcceptAll()
        return mockSender, mockSender
    }),
    fx.Decorate(func(demux demultiplexer.Mock, s sender.Sender) demultiplexer.Component {
        demux.SetDefaultSender(s)
        return demux
    }),
)
```

`Opts` provides two values via fx:

| Type | Description |
|---|---|
| `*mocksender.MockSender` | Raw mock, used to call `AssertMetric` / `AssertEventPlatformEvent` in tests |
| `sender.Sender` | Same object cast to the interface, used by components under test |

The `fx.Decorate` call replaces the default sender in the mock demultiplexer with the mock sender, so calls to `demux.GetDefaultSender()` inside the forwarder return the mock.

### What it bundles

- `defaultforwarder.MockModule()` — no-op forwarder, avoids real HTTP connections
- `demultiplexerimpl.MockModule()` — in-memory demultiplexer mock
- `hostnameimpl.MockModule()` — hostname provider stub
- `logmock.New(t)` — test logger that fails the test on unexpected error logs

## Usage

Add `senderhelper.Opts` to any `fxutil.Test[...]` call that tests a component that reads from a demultiplexer sender:

```go
// From forwarderimpl/forwarder_test.go
s := fxutil.Test[services](t,
    configimpl.MockModule(),
    senderhelper.Opts,          // provides MockSender + wires demux
    formatterimpl.MockModule(),
    listenerimpl.MockModule(),
    Module(),
)

// Inject a packet, then assert the forwarder emitted the right event:
s.Listener.Send(packet)
time.Sleep(100 * time.Millisecond)
s.Sender.AssertEventPlatformEvent(t, rawEvent, eventplatform.EventTypeSnmpTraps)
```

The `*mocksender.MockSender` is injectable into the test struct via fx because `Opts` provides it directly.

> Note: `senderhelper` is only used within the `comp/snmptraps` subtree. Do not add it to non-test builds.
