# Migrating to Components

After your component has been created you can link it to other components such as flares (other like status pages, or health will come later).

This page documents how to fully integrate your component in the Agent life cycle.

## Flare

Migrate the code related to your component's domain from `pkg/flare` to your component. After migrating
the Agent to components, you can delete it from `pkg/flare`.

### Creating a callback

To add data to a flare you will fiest need to register a `FlareBuilder`.

First create a `flare.go` file in your component (the file name is a convention) and create a `func (c *yourComp) fillFlare(fb flarehelpers.FlareBuilder) error` function.

This function is called every time the Agent generates a flare, either from the CLI or from the running Agent. This
callback receives a `comp/flare/flare/helpers:FlareBuilder`. The `FlareBuilder` interface provides the
helpers required to add data to a flare: adding files, copying directories, scrubbing data, and so on.

Example of `flare.go`:

```golang
import (
	yaml "gopkg.in/yaml.v2"

	flarehelpers "github.com/DataDog/datadog-agent/comp/flare/flare/helpers"
)

func (c *myComponent) fillFlare(fb flarehelpers.FlareBuilder) error {
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

### Register the callback

Finally, to Register your callback, provide a new `comp/flare/flare/helpers:Provider` by using
`comp/flare/flare/helpers:NewProvider`.

For this the constructor of your component must return a `helpers.Provider` that will be called for each flare creation
(`NewProvider` does all the underlying work for you).

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
	flarehelpers "github.com/DataDog/datadog-agent/comp/flare/flare/helpers"
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

### Migrating your code

The code in `pkg/flare` uses the `FlareBuilder` interface, simplifying migration. Locate the code
related to your component domain from `pkg/flare` and move it to your `fillFlare` function.
