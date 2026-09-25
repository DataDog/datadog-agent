# Test a component

Test the ZSTD implementation from the [creation tutorial](creating-components.md) through its public interface, using mocks for configuration and logging. Then build a separate example to test lifecycle hooks.

## Before you start

Complete the creation tutorial and keep its files under `comp/compression`. Run the commands below from the repository root with the [development tooling](../../setup/required.md#tooling) configured.

The creation tutorial tests Fx assembly. Here, you will call the implementation's constructor directly to test its behavior independently of Fx. Test any additional compression implementations separately.

## Test the compression interface

Create `comp/compression/impl-zstd/component_test.go`. Use the same `zstdimpl` package as the implementation so the test can call `NewComponent` with its `Requires` struct.

Supply configuration with `config.NewMock(t)` from `comp/core/config` and logging with `logmock.New(t)` from `comp/core/log/mock`. These mocks require the `test` build tag. As with the [compression mock](creating-components.md#add-a-mock-for-consumers), they let a test provide a component's dependencies without assembling an application.

Check that compression followed by decompression restores both ordinary text and empty input. Compare the restored bytes with the input; the exact compressed representation can vary with library versions and compression settings. Also check that decompression rejects invalid data.

/// tab | :octicons-file-code-16: comp/compression/impl-zstd/component_test.go
```go
//go:build test

package zstdimpl

import (
    "bytes"
    "testing"

    "github.com/stretchr/testify/require"

    config "github.com/DataDog/datadog-agent/comp/core/config"
    logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

func TestCompressionRoundTrip(t *testing.T) {
    tests := []struct {
        name  string
        input []byte
    }{
        {name: "text", input: []byte("Hello from a component")},
        {name: "empty", input: []byte{}},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            component := NewComponent(Requires{
                Conf: config.NewMock(t),
                Log:  logmock.New(t),
            }).Comp

            compressed, err := component.Compress(tt.input)
            require.NoError(t, err)
            restored, err := component.Decompress(compressed)
            require.NoError(t, err)
            require.True(t, bytes.Equal(tt.input, restored), "round trip changed the input bytes")
        })
    }
}

func TestDecompressInvalidData(t *testing.T) {
    component := NewComponent(Requires{
        Conf: config.NewMock(t),
        Log:  logmock.New(t),
    }).Comp

    _, err := component.Decompress([]byte("not a ZSTD frame"))
    require.Error(t, err)
}
```
///

Run the implementation tests:

```shell
dda inv test --targets=./comp/compression/impl-zstd --verbose
```

Expect both `TestCompressionRoundTrip` subtests and `TestDecompressInvalidData` to pass. You have now checked successful round trips, the empty-input boundary, and an error case through the public compression interface.

## Test lifecycle hooks

The compression implementation does not register lifecycle hooks. For this separate exercise, create the three files below under the illustrative path `comp/lifecycleexample` and replace `<your team>` with the owning team's name. The component exposes whether its startup hook has run and its shutdown hook has not yet run. An atomic flag keeps state queries safe during startup and shutdown.

First, define the public interface:

/// tab | :octicons-file-code-16: comp/lifecycleexample/def/component.go
```go
// Package lifecycleexample defines the interface for the lifecycle exercise.
package lifecycleexample

// team: <your team>

// Component reports the component's lifecycle state.
type Component interface {
    // IsRunning reports whether startup has completed and shutdown has not begun.
    IsRunning() bool
}
```
///

Next, implement the interface and register the hooks in the constructor. The constructor only registers the hooks; startup and shutdown change the running state.

/// tab | :octicons-file-code-16: comp/lifecycleexample/impl/component.go
```go
// Package lifecycleexampleimpl implements the lifecycle exercise.
package lifecycleexampleimpl

import (
    "context"
    "sync/atomic"

    compdef "github.com/DataDog/datadog-agent/comp/def"
    lifecycleexample "github.com/DataDog/datadog-agent/comp/lifecycleexample/def"
)

// Requires supplies the lifecycle used to register startup and shutdown hooks.
type Requires struct {
    Lifecycle compdef.Lifecycle
}

// Provides exposes the component's lifecycle state.
type Provides struct {
    Comp lifecycleexample.Component
}

type component struct {
    running atomic.Bool
}

// NewComponent registers the hooks without starting the component.
func NewComponent(reqs Requires) Provides {
    comp := &component{}
    reqs.Lifecycle.Append(compdef.Hook{
        OnStart: comp.start,
        OnStop:  comp.stop,
    })
    return Provides{Comp: comp}
}

func (c *component) start(_ context.Context) error {
    c.running.Store(true)
    return nil
}

func (c *component) stop(_ context.Context) error {
    c.running.Store(false)
    return nil
}

// IsRunning reports whether startup has completed and shutdown has not begun.
func (c *component) IsRunning() bool {
    return c.running.Load()
}
```
///

Finally, supply <<<repo("comp/def/lifecycle_mock.go", "`compdef.NewTestLifecycle(t)`", match="^func NewTestLifecycle")>>> directly to the constructor. This helper records the hooks and lets the test run startup and shutdown explicitly. It requires the `test` build tag and does not need an Fx application or lifecycle adapter.

/// tab | :octicons-file-code-16: comp/lifecycleexample/impl/component_test.go
```go
//go:build test

package lifecycleexampleimpl

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"

    compdef "github.com/DataDog/datadog-agent/comp/def"
)

func TestLifecycleHooks(t *testing.T) {
    lc := compdef.NewTestLifecycle(t)
    component := NewComponent(Requires{Lifecycle: lc}).Comp

    lc.AssertHooksNumber(1)
    require.False(t, component.IsRunning())

    ctx := context.Background()
    require.NoError(t, lc.Start(ctx))
    require.True(t, component.IsRunning())

    require.NoError(t, lc.Stop(ctx))
    require.False(t, component.IsRunning())
}
```
///

Run the lifecycle test:

```shell
dda inv test --targets=./comp/lifecycleexample/impl --verbose
```

Expect `TestLifecycleHooks` to pass. Its assertions verify that construction leaves the component stopped, startup makes it running, and shutdown stops it again. The test observes these changes through the public interface.

## Next steps

See how to [use components in a binary](../../how-to/components/using-components.md), or read about [Fx lifecycle behavior](../../architecture/components/fx.md#lifecycle) and the [component lifecycle requirements](../../guidelines/components.md#concurrency-and-lifecycle).
