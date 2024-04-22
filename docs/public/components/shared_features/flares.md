# Flare

The general idea is to register a callback within your component to be called each time a flare is created. This uses
[Fx](../fx.md) groups under the hood, but helpers are there to abstract all the complexity.

Once the callback is created you will have to migrate the code related to your component from `pkg/flare` to your
component.

## Creating a callback

To add data to a flare you will first need to register a callback, aka a `FlareBuilder`.

Within your component create a method with the following signature `func (c *yourComp) fillFlare(fb flaretypes.FlareBuilder) error`.

This function is called every time the Agent generates a flare, either from the CLI, RemoteConfig or from the running
Agent. Your callback takes a
[FlareBuilder](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/flare/types#FlareBuilder) as parameter.
This object provides all the helpers functions needed to add data to a flare (adding files, copying
directories, scrubbing data, and so on).

Example:

```golang
import (
	yaml "gopkg.in/yaml.v2"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

func (c *myComponent) fillFlare(fb flaretypes.FlareBuilder) error {
	// Creating a new file
	fb.AddFile(
		"runtime_config_dump.yaml",
		[]byte("content of my file"),
	)

	// Copying a file from the disk into the flare
	fb.CopyFile("/etc/datadog-agent/datadog.yaml")
	return nil
}
```

Read the [FlareBuilder](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/flare/types#FlareBuilder) package documentation for for more information on the API.

Any error returned by the `FlareBuilder` methods are already logged into a file shipped within the flare. This means, in
most cases, you can ignore errors returned by the `FlareBuilder` methods. In all cases, ship as much data as possible in a flare instead of stopping at the first error.

Returning an error from your callback does not stop the flare from being created or sent but will be logged into the
flare too.

While it's possible to register multiple callbacks from the same component, try to keep all the flare code in a single callback.

## Register your callback

Now you need to register your callback to be called each time a flare is created. To do so your component constructor
needs to provide a new [Provider](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/flare/types#Provider).
Use [NewProvider](https://pkg.go.dev/github.com/DataDog/datadog-agent/comp/core/flare/types#NewProvider) function for this.

Example:
```golang
import (
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

type Provides struct {
	// [...]

	// Declare that your component will return a flare provider
	FlareProvider flaretypes.Provider
}

func newComponent(deps Requires) Provides {
	// [...]

	return Provides{
		// [...]

		// NewProvider will wrap your callback in order to be use as a 'Provider'
		FlareProvider: flaretypes.NewProvider(myComponent.fillFlare),
	}, nil
}
```

## Testing

The flare component offers a
[FlareBuilder mock](https://github.com/DataDog/datadog-agent/blob/d0035f997e796204ec4ec07a8bc467c85b9ee6fb/comp/core/flare/helpers/builder_mock.go#L22) to test your callback.


Example:
```golang
import (
	"testing"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

func TestFillFlare(t testing.T) {
	myComp := newComponent(...)

	flareBuilderMock := helpers.NewFlareBuilderMock(t)

	myComp.fillFlare(flareBuilderMock, false)
	
	flareBuilderMock.AssertFileExists("datadog.yaml")
	flareBuilderMock.AssertFileContent("some_file.txt", "my content")
	// ...
}
```

## Migrating your code

Now comes the hard part: migrating the code from
[pkg/flare](https://pkg.go.dev/github.com/DataDog/datadog-agent/pkg/flare) related to you component to your new
callback.

The good news is that the code in `pkg/flare` already uses the `FlareBuilder` interface. So you shouldn't need to
rewrite any logic. Don't forget to migrate the tests too and expand them (most of the flare features are not tested).

Keep in mind that the goal is to delete `pkg/flare` once the migration to component is done.
