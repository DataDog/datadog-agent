# FAQ

TODO: Should we migrate the FAQ from confluence here ?

## Optional dependency

You might need to express the fact that some of your dependencies are optional. This often happens for
components that interact with many other components **if available** (ie: were included at compile time). This allow
your component to interact with other without forcing their inclusion in the current binary.

The [optional.Option](https://github.com/DataDog/datadog-agent/tree/main/pkg/util/optional) type answer such need.

A good example would our metadata components that are included in multiple binaries (`core-agent`, `DogStatsD`, ...).
Such components want to use the `sysprobeconfig` component if available. `sysprobeconfig` is available in the
`core-agent` but not in `DogStatsD`.

To do this the `metadata` component will do this:

```
type Requires struct {
    SysprobeConf optional.Option[sysprobeconfig.Component]
    [...]
}

func NewMetadata(deps Requires) (metadata.Component) {
    if sysprobeConf, available := deps.SysprobeConf.Get(); available {
        // interact with sysprobeconfig
    }
}
```

The above code would produce a generique component, included in both `core-agent` and `DogStatsD` binaries, that **can**
interact with `sysprobeconfig` without forcing the binaries to compile with it.

This pattern can be used for every components since they all provide a convertion function to `Fx` to convert their
`Component` interface to `optional.Option[Component]` (see [creating components](creating-components.md)).


