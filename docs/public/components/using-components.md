# Using components

Using components within other components is covered on the [create components page](creating-components.md).

Now let's explore how to use components in your binaries. One of the core idea behind component design is to be able to
create new binaries for customers by aggregating components.

## the `cmd` folder

All `main` functions and binary entry points should be in the `cmd` folder.

The `cmd` folder uses the following hierarchy:

```
cmd /
    <binary name> /
        main.go                   <-- The entry points from your binary
        subcommands /             <-- All subcommand for your binary CLI
            <subcommand name> /   <-- The code specific to a single subcommand
                command.go
                command_test.go
```

Say you want to add a `test` command to the `agent` CLI.

You would create the following file:

=== ":octicons-file-code-16: cmd/agent/subcommands/test/command.go"
    ```go
    package test

    import (
    // [...]
    )

    // Commands returns a slice of subcommands for the 'agent' command.
    //
    // The Agent uses "cobra" to create its CLI. The command method is your entrypoint. Here, you're going to create a single
    // command.
    func Commands(globalParams *command.GlobalParams) []*cobra.Command {
        cmd := &cobra.Command{
            Use:   "test",
            Short: "a test command for the Agent",
            Long:  ``,
            RunE: func(_ *cobra.Command, _ []string) error {
                return fxutil.OneShot(
                    <callback>,
                    <list of dependencies>.
                )
            },
        }

        return []*cobra.Command{cmd}
    }
    ```

The code above creates a test command that does nothing. As you can see, `fxutil.OneShot` helpers are being used. These
helpers initialize an Fx app with all the wanted dependencies. 

The next section explains how to request a
dependency.

## Importing components

The `fxutil.OneShot` takes a list of components and gives them to Fx. Note that this only tells Fx how to create types when they're needed. This does not do anything else.

For a component to be instantiated, it must be one of the following:

+ Required as a parameter by the `callback` function
+ Required as a dependency from other components already marked for instantiation
+ Directly asked for by using `fx.Invoke`. More on this on the [Fx page](fx.md).

Let's require the `log` components:

```go
import (
    // First let's import the FX wrapper to require it
    logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
    // Then the logger interface to use it
    log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// [...]
    return fxutil.OneShot(
        myTestCallback, // The function to call from fxutil.OneShot
        logfx.Module(), // This will tell FX how to create the `log.Component`
    )
// [...]

func myTestCallback(logger log.Component) {
    logger.Info("some message")
}
```

## Importing bundles

Now let's say you want to include the core bundle instead. The core bundle offers many basic features (logger, config,
telemetry, flare, ...).

```go
import (
    // We import the core bundle
    core "github.com/DataDog/datadog-agent/comp/core"

    // Then the interfaces we want to use
    config "github.com/DataDog/datadog-agent/comp/core/config/def"
)

// [...]
    return fxutil.OneShot(
        myTestCallback, // The function to call from fxutil.OneShot
        core.Bundle(),  // This will tell FX how to create the all the components included in the bundle
    )
// [...]

func myTestCallback(conf config.Component) {
    api_key := conf.GetString("api_key")

    // [...]
}
```

It's very important to understand that since `myTestCallback` only uses the `config.Component`, not all components from
the `core` bundle are instantiated! The `core.Bundle` instructs Fx how to create components, but only the ones required
are created.

In our example, the `config.Component` might have dozens of dependencies instantiated from the core bundle. Fx handles
all of this.

## Using plain data types with Fx

As your migration to components is not finished, you might need to manually instruct Fx on how to use plain types.

You will need to use `fx.Supply` for this. More details can be found [here](fx.md).

But here is a quick example:

```go
import (
    logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
    log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// plain custom type
type custom struct {}

// [...]
    return fxutil.OneShot(
        myTestCallback,
        logfx.Module(),

        // fx.Supply populates values into Fx. 
        // Any time this is needed, Fx will use it.
        fx.Supply(custom{})
    )
// [...]

// Here our function uses component and non-component type, both provided by Fx.
func myTestCallback(logger log.Component, c custom) {
    logger.Info("Custom type: %v", c)
}
```

!!! Info
    This means that components can depend on plain types too (as long as the main entry point populates Fx options with them).
    
<!-- TODO: Provide an exmaple using fx.Provide -->

<!-- TODO: 
## Using components parameters -->
