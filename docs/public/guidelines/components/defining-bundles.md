# Defining Component Bundles

A bundle is defined in a dedicated package named `comp/<bundleName>`. The package must have the following defined in `bundle.go`:

* Extensive package-level documentation. This should define:
    * The purpose of the bundle
    * What components are and are not included in the bundle. Components might be omitted in the interest of binary size, as discussed in the [component overview](../../architecture/components/overview.md).
    * Which components are automatically instantiated.
    * Which other _bundles_ this bundle depends on. Bundle dependencies are always expressed at a bundle level.
* A team-name comment of the form `// team: <teamname>`. This is used to generate CODEOWNERS information.
* An optional `BundleParams` -- the type of the bundle's parameters (see below). This item should have a formulaic doc string like `// BundleParams defines the parameters for this bundle.`
* `Bundle` -- an `fx.Option` that can be included in an `fx.App` to make this bundle's components available. To assist with debugging, use `fxutil.Bundle(options...)`. Use `fx.Invoke(func(componentpkg.Component) {})` to instantiate components automatically. This item should have a formulaic doc string like `// Module defines the fx options for this component.`

Typically, a bundle will automatically instantiate the top-level components that represent the bundle's purpose. For example, the trace-agent bundle `comp/trace` might automatically instantiate `comp/trace/agent`.

You can use the invoke task `inv components.new-bundle comp/<bundleName>` to generate a pre-filled `bundle.go` file for the given bundle.

## Bundle Parameters

Apps can provide some intialization-time parameters to bundles. These parameters are limited to two kinds:

* Parameters specific to the app, such as whether to start a network server; and
* Parameters from the environment, such as command-line options.

Anything else is runtime configuration and should be handled vi `comp/core/config` or another mechanism.

Bundle parameters must stored only `Params` types for sub components. The reason is that each sub component
must be usable without `BundleParams`.

=== ":octicons-file-code-16: comp/&lt;bundleName&gt;/bundle.go"
    ```go
    import ".../comp/<bundleName>/foo"
    import ".../comp/<bundleName>/bar"
    // ...

    // BundleParams defines the parameters for this bundle.
    type BundleParams struct {
        Foo foo.Params
        Bar bar.Params
    }

    var Bundle = fxutil.Bundle(
        // You must tell to fx how to get foo.Params from BundleParams.
        fx.Provide(func(params BundleParams) foo.Params { return params.Foo }),
        foo.Module(),
        // You must tell to fx how to get bar.Params from BundleParams.
        fx.Provide(func(params BundleParams) bar.Params { return params.Bar }),
        bar.Module(),
    )
    ```

## Testing

A bundle should have a test file, `bundle_test.go`, to verify the documentation's claim about its dependencies. This simply uses `fxutil.TestBundle` to check that all dependencies are satisfied when given the full set of required bundles.

=== ":octicons-file-code-16: bundle_test.go"
    ```go
    func TestBundleDependencies(t *testing.T) {
        fxutil.TestBundle(t, Bundle)
    }
    ```
