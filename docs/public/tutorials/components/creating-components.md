# Create a component

Build a component that compresses and decompresses bytes with ZSTD, register it with Fx, and run a test that restores the original bytes. Along the way, you will create separate packages for the interface, implementation, Fx wrapper, and mock. The component is designed for multiple compression algorithms; this exercise builds its ZSTD variant.

## Before you start

Start at the root of an Agent checkout with the [development tooling](../../setup/required.md#tooling) configured. The examples use the illustrative path `comp/compression`; create the files at the paths shown below and replace `<your team>` with the owning team's name. All Go files shown in this tutorial are complete files.

The [component framework overview](../../architecture/components/index.md) explains the package boundaries. Keep the [component guidelines](../../guidelines/components.md) available for the repository's conventions.

## Define the interface

Create `comp/compression/def/component.go` with the two operations consumers will use:

/// tab | :octicons-file-code-16: comp/compression/def/component.go
```go
// Package compression defines the interface shared by compression implementations.
package compression

// team: <your team>

// Component describes the interface implemented by all compression implementations.
type Component interface {
    // Compress compresses the input data.
    Compress([]byte) ([]byte, error)

    // Decompress decompresses the input data.
    Decompress([]byte) ([]byte, error)
}
```
///

Consumers can now name the compression interface without importing a compression algorithm.

## Implement ZSTD compression

Create `comp/compression/impl-zstd/compressor.go`. Its `Requires` struct asks for configuration and logging, and its `Provides` struct exposes the compression interface. Both structs are plain Go types with exported fields.

The implementation uses the repository's pure-Go ZSTD library and `serializer_zstd_compressor_level` setting. For this exercise, each call creates and closes its encoder or decoder to keep cleanup local, at the cost of repeated initialization. `EncodeAll` and `DecodeAll` also support concurrent reuse; the <<<repo("pkg/zstd/zstd_nocgo.go", "production pure-Go backend")>>> also initializes a new encoder or decoder on every call.

/// tab | :octicons-file-code-16: comp/compression/impl-zstd/compressor.go
```go
// Package zstdimpl compresses and decompresses buffers using ZSTD.
package zstdimpl

import (
    "github.com/klauspost/compress/zstd"

    compression "github.com/DataDog/datadog-agent/comp/compression/def"
    config "github.com/DataDog/datadog-agent/comp/core/config"
    log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// Requires supplies the compression settings and logger.
type Requires struct {
    Conf config.Component
    Log  log.Component
}

// Provides exposes the compression interface to consumers.
type Provides struct {
    Comp compression.Component
}

type compressor struct {
    conf config.Component
    log  log.Component
}

// NewComponent returns a ZSTD implementation of the compression interface.
func NewComponent(reqs Requires) Provides {
    comp := &compressor{
        conf: reqs.Conf,
        log:  reqs.Log,
    }
    return Provides{
        Comp: comp,
    }
}

// Compress compresses the input data using ZSTD.
func (c *compressor) Compress(data []byte) ([]byte, error) {
    c.log.Debug("compressing a buffer with ZSTD")
    encoder, err := zstd.NewWriter(nil,
        zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(c.conf.GetInt("serializer_zstd_compressor_level"))),
        // Empty input must still produce a valid ZSTD frame.
        zstd.WithZeroFrames(true),
    )
    if err != nil {
        return nil, err
    }
    defer encoder.Close()
    return encoder.EncodeAll(data, nil), nil
}

// Decompress decompresses the input data using ZSTD.
func (c *compressor) Decompress(data []byte) ([]byte, error) {
    c.log.Debug("decompressing a buffer with ZSTD")
    decoder, err := zstd.NewReader(nil)
    if err != nil {
        return nil, err
    }
    defer decoder.Close()
    return decoder.DecodeAll(data, nil)
}
```
///

The constructor now returns a value implementing the interface. Compression and decompression report errors through their method results. The implementation contains no Fx imports; the next file adapts its constructor for Fx.

## Register the component with Fx

Create `comp/compression/fx-zstd/fx.go` to register the constructor and an optional conversion:

/// tab | :octicons-file-code-16: comp/compression/fx-zstd/fx.go
```go
// Package fx registers ZSTD compression with Fx.
package fx

import (
    compression "github.com/DataDog/datadog-agent/comp/compression/def"
    compressionimpl "github.com/DataDog/datadog-agent/comp/compression/impl-zstd"
    "github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module registers ZSTD compression and its optional conversion.
func Module() fxutil.Module {
    return fxutil.Component(
        fxutil.ProvideComponentConstructor(
            compressionimpl.NewComponent,
        ),
        fxutil.ProvideOptional[compression.Component](),
    )
}
```
///

This wrapper makes both `compression.Component` and `option.Option[compression.Component]` available to an application. The application also needs providers for configuration and logging; the completion test below supplies mocks for them.

/// info | Optional dependencies
`fxutil.ProvideOptional[T]()` registers a conversion from a provided `T` to `option.Option[T]`, using `github.com/DataDog/datadog-agent/pkg/util/option`. The component scaffold includes this registration by default. When writing a wrapper yourself, include it if the component has optional consumers. The conversion still requires a provider for `T`; it does not create an empty option when that provider is missing.

See [optional dependencies](../../how-to/components/optional-dependencies.md#declare-the-optional-dependency) for a consumer and a wrapper that explicitly supplies an absent value.
///

## Add a mock for consumers

Create `comp/compression/mock/mock.go` following the [mock conventions](../../guidelines/components.md#mocks), so consumer tests can substitute fixed responses for compression. Its methods return fixed byte strings, and its `test` build tag makes it available in the repository's unit-test workflow. The completion test will use the real ZSTD implementation to check a round trip.

/// tab | :octicons-file-code-16: comp/compression/mock/mock.go
```go
//go:build test

// Package mock provides fixed compression responses for consumer tests.
package mock

import (
    "testing"

    compression "github.com/DataDog/datadog-agent/comp/compression/def"
)

// Provides exposes the mock through the compression interface.
type Provides struct {
    Comp compression.Component
}

type mock struct{}

// New returns a mock compressor with fixed responses.
func New(*testing.T) Provides {
    return Provides{
        Comp: &mock{},
    }
}

// Compress returns a fixed response regardless of the input.
func (c *mock) Compress(_ []byte) ([]byte, error) {
    return []byte("compressed"), nil
}

// Decompress returns a fixed response regardless of the input.
func (c *mock) Decompress(_ []byte) ([]byte, error) {
    return []byte("decompressed"), nil
}
```
///

In a consumer test, import `github.com/DataDog/datadog-agent/comp/compression/mock` as `compressionmock` and pass `compressionmock.New(t).Comp` as the compression dependency. The [optional-dependency guide](../../how-to/components/optional-dependencies.md#register-and-verify-the-selected-provider) shows how to use this mock for the present case and check the absent case separately.

## Assemble and exercise the component

Create `comp/compression/fx-zstd/fx_test.go`. This test assembles the wrapper with configuration and logging mocks, requests the compression interface and its optional conversion, and checks that decompressing a compressed payload restores the original bytes.

/// tab | :octicons-file-code-16: comp/compression/fx-zstd/fx_test.go
```go
//go:build test

package fx_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "go.uber.org/fx"
    "go.uber.org/fx/fxtest"

    compression "github.com/DataDog/datadog-agent/comp/compression/def"
    compressionfx "github.com/DataDog/datadog-agent/comp/compression/fx-zstd"
    config "github.com/DataDog/datadog-agent/comp/core/config"
    log "github.com/DataDog/datadog-agent/comp/core/log/def"
    logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
    "github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestCompressionRoundTrip(t *testing.T) {
    var compressor compression.Component
    var optionalCompressor option.Option[compression.Component]
    app := fxtest.New(t,
        compressionfx.Module(),
        fx.Provide(func() config.Component { return config.NewMock(t) }),
        fx.Provide(func() log.Component { return logmock.New(t) }),
        fx.Populate(&compressor, &optionalCompressor),
    )
    app.RequireStart()
    t.Cleanup(func() { app.RequireStop() })

    present, found := optionalCompressor.Get()
    require.True(t, found)
    require.Same(t, compressor, present)

    input := []byte("Hello from a component")
    compressed, err := compressor.Compress(input)
    require.NoError(t, err)
    restored, err := compressor.Decompress(compressed)
    require.NoError(t, err)
    require.Equal(t, input, restored)
    t.Logf("Round trip restored %q", restored)
}
```
///

Run the test from the repository root:

```shell
dda inv test --targets "./comp/compression/..." --test-run-name TestCompressionRoundTrip --verbose
```

The command supplies the repository's build tags, including `test`. Expect `TestCompressionRoundTrip` to pass and its output to include `Round trip restored "Hello from a component"`. You have now constructed the real implementation through Fx and called it through its public interface.

## Final state

Your completed component has these files:

```text
comp/compression/
├── def
│   └── component.go
├── fx-zstd
│   ├── fx_test.go
│   └── fx.go
├── impl-zstd
│   └── compressor.go
└── mock
    └── mock.go
```

## Next steps

For future components, `dda inv components.new-component comp/<COMPONENT_NAME>` creates the default `def`, `impl`, `fx`, and `mock` layout. The [package guidelines](../../guidelines/components.md#file-hierarchy) describe how to add alternate implementations such as ZIP.

Continue with [component testing](testing.md), [using components in a binary](../../how-to/components/using-components.md), or [adding an optional dependency](../../how-to/components/optional-dependencies.md). If the component needs to be imported outside the repository, consult the [module boundaries](../../guidelines/components.md#go-modules) and [nested module guide](../../how-to/go/modules.md).
