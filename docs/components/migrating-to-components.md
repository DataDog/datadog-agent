# Migrating to Components

After your component has been created you can link it to other components such as flares, status pages, or health.

This page documents how to fully integrate your component in the Agent life cycle.

## Flare

Migrate the code related to your component's domain from `pkg/flare` to your component. After migrating
the Agent to components, you can delete `pkg/flare`.

### Creating a callback

To add data to a flare, register a provider.

First create a `flare.go` file in your component (the file name is a convention) and create a `func (c *yourComp) fillFlare(fb flarehelpers.FlareBuilder) error` function.

This function is called every time the Agent generates a flare, either from the CLI or from the running Agent. This
callback receives a `comp/core/flare/helpers:FlareBuilder`. The `FlareBuilder` interface provides the
helpers required to add data to a flare: adding files, copying directories, scrubbing data, and so on.

Example of `flare.go`:

```golang
import (
	yaml "gopkg.in/yaml.v2"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

func (c *cfg) fillFlare(fb flarehelpers.FlareBuilder) error {
	fb.AddFileFromFunc(
		"runtime_config_dump.yaml",
		func () ([]byte, error) {
			return yaml.Marshal(c.AllSettings())
		},
	)

	fb.CopyFile("/etc/datadog-agent/datadog.yaml")
	return nil
}
```

Read the package documentation for `FlareBuilder` for more information. 
All errors are automatically 
added to a log file shipped within the flare. Ship as much data as possible in a flare instead of
stopping at the first error. Returning an error does not stop the flare from being created or sent.

### Migrating your code

The code in `pkg/flare` uses the `FlareBuilder` interface, simplifying migration. Locate the code
related to your component domain from `pkg/flare` and move it to your `fillFlare` function.

### Register the callback

Finally, to Register your callback, provide a new `comp/core/flare/helpers:Provider` by using `comp/core/flare/helpers:NewProvider`.

Example from the `config` component:

In `component.go`:
```golang
// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newConfig),
)
```

In `config.go`:
```golang
import (
	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

type provides struct {
	fx.Out

	// [...]
	FlareProvider flarehelpers.Provider
	// [...]
}

func newConfig(deps dependencies) (provides, error) {
	// [...]
	return provides{
		// [...]
		FlareProvider: flarehelpers.NewProvider(myComponent.fillFlare),
		// [...]
	}, nil
}
```
