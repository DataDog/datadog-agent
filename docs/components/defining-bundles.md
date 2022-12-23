# Defining Bundles

A bundle is defined in a dedicated package named `comp/<bundleName>`.
The package must have the following defined in `bundle.go`:

 * Extensive package-level documentation.
   This should define:

     * The purpose of the bundle
     * What components are and are not included in the bundle.
       Components might be omitted in the interest of binary size, as discussed in the [overview](./components.md).
     * Which components are automatically instantiated.
     * Which other _bundles_ this bundle depends on.
       Bundle dependencies are always expressed at a bundle level.

 * A team-name comment of the form `// team: <teamname>`.
   This is used to generate CODEOWNERS information.

 * `BundleParams` -- the type of the bundle's parameters (see below).
   This item should have a formulaic doc string like `// BundleParams defines the parameters for this bundle.`

 * `Bundle` -- an `fx.Option` that can be included in an `fx.App` to make this bundle's components available.
   To assist with debugging, use `fxutil.Bundle(options...)`.
   Use `fx.Invoke(func(componentpkg.Component) {})` to instantiate components automatically.
   This item should have a formulaic doc string like `// Module defines the fx options for this component.`

Typically, a bundle will automatically instantiate the top-level components that represent the bundle's purpose.
For example, the trace-agent bundle `comp/trace` might automatically instantiate `comp/trace/agent`.

## Bundle Parameters

Apps can provide some intialization-time parameters to bundles.
These parameters are limited to two kinds:

 * Parameters specific to the app, such as whether to start a network server; and
 * Parameters from the environment, such as command-line options.

Anything else is runtime configuration and should be handled vi `comp/core/config` or another mechanism.

To avoid Go package cycles, the `BundleParams` type must be defined in the bundle's internal package, and re-exported from the bundle package:

```go
// --- comp/<bundleName>/internal/params.go ---

// BundleParams defines the parameters for this bundle.
type BundleParams struct {
    ...
}
```

```go
// --- comp/<bundleName>/bundle.go ---
import ".../comp/<bundleName>/internal"
import ".../comp/<bundleName>/foo"
// ...

// BundleParams defines the parameters for this bundle.
type BundleParams = internal.BundleParams

var Bundle = fxutil.Bundle(
    foo.Module,
)
```

Components within the bundle can then require `internal.BundleParams` and modify their behavior appropriately:

```go
// --- comp/<bundleName>/foo/foo.go

func newFoo(..., params internal.BundleParams) provides {
    if params.HyperMode { ... }
}
```

## Testing

A bundle should have a test file, `bundle_test.go`, to verify the documentation's claim about its dependencies.
This simply uses ValidateApp to check that all dependencies are satisfied when given the full set of required bundles.

```go
func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		fx.Supply(core.BundleParams{}),
		core.Bundle,
		fx.Supply(autodiscovery.BundleParams{}),
		autodiscovery.Bundle,
		fx.Supply(BundleParams{}),
		Bundle))
}
```
