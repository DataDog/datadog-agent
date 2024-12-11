# FAQ

<!-- TODO: Should we migrate the FAQ from confluence here ? -->

## Optional Component

You might need to express the fact that some of your dependencies are optional. This often happens for
components that interact with many other components **if available** (that is, if they were included at compile time). This allows
your component to interact with each other without forcing their inclusion in the current binary.

The [optional.Option](https://github.com/DataDog/datadog-agent/tree/main/pkg/util/optional) type answers this need.

For examples, consider the metadata components that are included in multiple binaries (`core-agent`, `DogStatsD`, etc.).
These components use the `sysprobeconfig` component if it is available. `sysprobeconfig` is available in the
`core-agent` but not in `DogStatsD`.

To do this in the `metadata` component:

```
type Requires struct {
    SysprobeConf optional.Option[sysprobeconfig.Component]
    [...]
}

func NewMetadata(deps Requires) (metadata.Component) {
    if sysprobeConf, found := deps.SysprobeConf.Get(); found {
        // interact with sysprobeconfig
    }
}
```

The above code produces a generic component, included in both `core-agent` and `DogStatsD` binaries, that **can**
interact with `sysprobeconfig` without forcing the binaries to compile with it.

You can use this pattern for every component, since all components provide Fx with a conversion function to convert their
`Component` interfaces to `optional.Option[Component]` (see [creating components](creating-components.md)).


