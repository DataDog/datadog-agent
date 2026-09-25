# Create a component bundle

Use a bundle to include related components together. See the [bundle overview](../../architecture/components/index.md#bundles) for when to use one.

## Creating a bundle

A bundle eases the aggregation of multiple components and lives in `comp/<bundlesName>/`.

/// tab | :octicons-file-code-16: comp/&lt;bundleName&gt;/bundle.go
```go
// Package <bundleName> ...
package <bundleName>

import (
    "github.com/DataDog/datadog-agent/pkg/util/fxutil"

    // We import all the components that we want to aggregate. A bundle must only aggregate components within its
    // sub-folders.
    comp1fx "github.com/DataDog/datadog-agent/comp/<bundleName>/comp1/fx"
    comp2fx "github.com/DataDog/datadog-agent/comp/<bundleName>/comp2/fx"
    comp3fx "github.com/DataDog/datadog-agent/comp/<bundleName>/comp3/fx"
    comp4fx "github.com/DataDog/datadog-agent/comp/<bundleName>/comp4/fx"
)

// A single team must own the bundle, even if they don't own all the sub-components
// team: <the team owning the bundle>

// Bundle defines the fx options for this bundle.
func Bundle() fxutil.BundleOptions {
    return fxutil.Bundle(
        comp1fx.Module(),
        comp2fx.Module(),
        comp3fx.Module(),
        comp4fx.Module(),
    )
}
```
///

A bundle doesn't need to import all sub components. The idea is to offer a default, easy to use grouping of components. But nothing prevents users from cherry-picking the components they want to use.

<!-- ## Bundle level params

TODO: write how to create level bundle params. -->
