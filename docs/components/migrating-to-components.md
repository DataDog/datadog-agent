# Migrating to Components

After your component has been created you can link it to other components such as flares (other like status pages, or health will come later).

This page documents how to fully integrate your component in the Agent life cycle.

## Flare

The general idea is to register a callback within your component to be called each time a flare is created. This use fx
groups under the hood, but helpers are there to abstract all of that for you.

Then, migrate the code related to your component's domain from `pkg/flare` to your component and delete it from `pkg/flare` once done.

### Creating a callback

To add data to a flare you will first need to register a callback, aka a `FlareBuilder`.

Within your component create a method with the following signature `func (c *yourComp) fillFlare(fb flaretypes.FlareBuilder) error`.

This function is called every time the Agent generates a flare, either from the CLI or from the running Agent. This
callback receives a `comp/core/flare/helpers:FlareBuilder`. The `FlareBuilder` interface provides all the
helpers functions needed to add data to a flare (adding files, copying directories, scrubbing data, and so on).

Example:

```golang
import (
	yaml "gopkg.in/yaml.v2"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

func (c *myComponent) fillFlare(fb flaretypes.FlareBuilder) error {
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

Read the package documentation for `FlareBuilder` for more information on the API.

All errors returned by the `FlareBuilder` are automatically added to a log file shipped within the flare. Ship as much
data as possible in a flare instead of stopping at the first error. Returning an error does not stop the flare from
being created or sent.

While you can register multiple callbacks from the same component, keep all the flare code in a single callback.

### Register your callback

Now you need to register you callback to be called each time a flare is created. To do so your component constructor
need to provide a new `comp/core/flare/helpers:Provider`. Use `comp/core/flare/helpers:NewProvider` for this.

Example:
```golang
import (
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

type provides struct {
	fx.Out

	// [...]
	FlareProvider flaretypes.Provider // Your component will provides a new FlareProvider
	// [...]
}

func newComponent(deps dependencies) (provides, error) {
	// [...]
	return provides{
		// [...]
		FlareProvider: flaretypes.NewProvider(myComponent.fillFlare), // NewProvider will wrap your callback in order to be use as a 'FlareProvider'
		// [...]
	}, nil
}
```

### Migrating your code

Now migrate the require code from `pkg/flare` to you component callback. The code in `pkg/flare` already uses the
`FlareBuilder` interface, simplifying migration. Don't forget to migrate the tests too and expand them (most of the
flare features are not tested). `comp/core/flare/helpers::NewFlareBuilderMock` will provides helpers for your tests.

Keep in mind that the goal is to delete `pkg/flare` once the migration to component is done.
