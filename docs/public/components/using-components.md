# Using components

We already cover using components within other components in the [create components page](creating-components.md).

Now let's explore how to use them in your binaries. One of the core idea behind the components design is to be able to
easily create new binaries for customers by aggregating components.

## the `cmd` folder

All `main` function and binary entry points should be in the `cmd` folder.

The `cmd` folder use the following hierarchy:

```
cmd /
    <binary name> /
        main.go                   <-- The entry points from your binary
        subcommands /             <-- All subcommand for your binary CLI
            <subcommand name> /   <-- The code specific to a single subcommand
                command.go
                command_test.go
```

Let's say we add a `test` command in `agent` CLI.

We would create the following file:

=== ":octicons-file-code-16: cmd/agent/subcommands/test/command.go"
    ```go
    package test

    import (
    // [...]
    )

    // Commands returns a slice of subcommands for the 'agent' command.
    //
    // The agent uses "cobra" to create its CLI. The command method is your entrypoint. Here we're going to create a single
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

The code above creates a test command that does nothing. You can see we're using the `fxutil.OneShot` helpers. This
helpers will initialize an `Fx` app for us with all the wanted dependencies. The next section explain how to request a
dependency.

## Importing components

The `fxutil.OneShot` will take a list of components and give them to `Fx`. It's important to understanding that this
will only tell `Fx` how to create types when they're needed. This alone will do nothing more.

In order for a components to be instantiated it needs to be required:

+ As a parameter for the `callback` function
+ As dependencies from other components already marked for instantiation
+ Directly asked for by using `fx.Invoke`. More on this in [fx page](fx.md).

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

Now let's say we want to include the core bundle instead. The core bundle offers many basic features (logger, config,
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

It's very important to understand that since `myTestCallback` only uses the `config.Component` not all components from
the `core` bundle were instantiated ! The `core.Bundle` instructs `Fx` how to create components but only the ones required
are created.

In our example, the `config.Component` might have dozens of dependencies instantiated from the core bundle. `Fx` handles
all of this for us.

## Using non-component types with Fx

As our migration to components is not finished you might need to manually instruct `Fx` on how to create certain types.

You will need to use `fx.Provide` for this. More details can be found [here](fx.md).

But here is a quick example:

```go
import (
    logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
    log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// a non-component type
type custom struct {}

// [...]
    return fxutil.OneShot(
        myTestCallback,
        logfx.Module(),

        // fx.Provide registers a function providing a type into Fx. Any time this is needed, Fx will use it.
        fx.Provide(func() custom {
            return custom{}
        }),
    )
// [...]

// Here our function uses component and non-component type, both provided by Fx.
func myTestCallback(logger log.Component, c custom) {
    logger.Info("Custom type: %v", c)
}
```

!!! Info
    This means that components can depend on non components type too (as long as the main instruct Fx how to create them).

## Using components parameters

TODO
