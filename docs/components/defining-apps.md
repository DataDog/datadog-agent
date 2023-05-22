# Defining Apps and Binaries

## Binaries

Each binary is defined as a `main` package in the `cmd/` directory, such as `cmd/iot-agent`.
This top-level package contains _only_ a simple `main` function (or often, one for windows and one for *nix) which performs any platform-specific initialization and then creates and executes a Cobra command.

### Binary Size

Consider carefully the tree of Go imports that begins with the `main` package.
While the Go linker does some removal of unused symbols, the safest means to ensure a particular package isn't occuping space in the resulting binary is to not include it.

### Simple Binaries

A 'simple binary" here is one that does not have subcommands.

The Cobra configuration for the binary is contained in the `command` subpackage of the main package (`cmd/<binary>/command`).
The `main` function calls this package to create the command, and then executes it:

```go
// cmd/<binary>/main.go
func main() {
	if err := command.MakeCommand().Execute(); err != nil {
		os.Exit(-1)
	}
}
```

The `command.MakeCommand` function creates the `*cobra.Command` for the binary, with a `RunE` field that defines an app, as described below.

### Binaries With Subcommands

Many binaries have a collection of subcommands, along with some command-line flags defined at the binary level.
For example, the `agent` binary has subcommands like `agent flare` or `agent diagnose` and accepts global `--cfgfile` and `--no-color` arguments.

As with simple binaries, the top-level Cobra command is defined by a `MakeCommand` function in `cmd/<binary>/command`.
This `command` package should also define a `GlobalParams` struct and a `SubcommandFactory` type:

```go
// cmd/<binary>/command/command.go
// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	ConfFilePath string

    // ...
}

// SubcommandFactory is a callable that will return a slice of subcommands.
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command
```

Each subcommand is implemented in a subpackage of `cmd/<binary>/subcommands`, such as `cmd/<binary>/subcommands/version`.
Each such subpackage contains a `command.go` defining a `Commands` function that defines the subcommands for that package:

```go
// cmd/<binary>/subcommands/<command>/command.go
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
    cmd := &cobra.Command { .. }
    return []*cobra.Command{cmd}
}
```

While `Commands` typically returns only one command, it may make sense to return multiple commands when the implementations share substantial amounts of code, such as starting, stopping and restarting a service.

The `main` function supplies a slice of subcommand factories to `command.MakeCommand`, which calls each one and adds the resulting subcommands to the root command.

```go
// cmd/<binary>/main.go
subcommandFactories := []command.SubcommandFactory{
    frobnicate.Commands,
    ...,
}
if err := command.MakeCommand(subcommandFactories).Execute(); err != nil {
    os.Exit(-1)
}
```

The `GlobalParams` type supports Cobra arguments that are global to all subcommands.
It is passed to each subcommand factory so that the defined `RunE` callbacks can access these arguments.
If the binary has no global command-line arguments, it's OK to omit this type.

```go
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	globalParams := GlobalParams{}

	cmd := &cobra.Command{ ... }
	cmd.PersistentFlags().StringVarP(
        &globalParams.ConfFilePath, "cfgpath", "c", "",
        "path to directory containing datadog.yaml")

	for _, sf := range subcommandFactories {
		subcommands := sf(&globalParams)
		for _, cmd := range subcommands {
			agentCmd.AddCommand(cmd)
		}
	}

	return cmd
}
```

If the available subcommands depend on build flags, move the creation of the subcommand factories to the
`subcommands/<command>` package and create the slice there using source files with `//go:build` directives. Your
factory can return `nil` if your command is not compatible with the current build flag. In all cases, the subcommands
build logic should be constrained to its package. See `cmd/agent/subcommands/jmx/command_nojmx.go` for an example.

## Apps

Apps map directly to `fx.App` instances, and as such they define a set of provided components and instantiate some of them.

The `fx.App` is always created _after_ Cobra has parsed the command-line, within a [`cobra.Command#RunE` function](https://pkg.go.dev/github.com/spf13/cobra#Command).
This means that the components supplied to an app, and any BundleParams values, are specific to the invoked command or subcommand.

### One-Shot Apps

A one-shot app is one which performs some task and exits, such as `agent status`.
The `pkg/util/fxutil.OneShot` helper function provides a convenient shorthand to run a function only after all components have started.
Use it like this:

```go
cmd := cobra.Command{
    Use: "foo", ...,
    RunE: func(cmd *cobra.Command, args []string) error {
        return fxutil.OneShot(run,
            fx.Supply(core.BundleParams{}),
            core.Bundle,
            ..., // any other bundles needed for this app
        )
    },
}

func run(log log.Component) error {
    log.Debug("foo invoked!")
    ...
}
```

The `run` function typically also needs some command-line values.
To support this, create a (sub)command-specific `cliParams` type containing the required values, and embedding a pointer to GlobalParams:

```go
type cliParams struct {
    *command.GlobalParams
    useTLS bool
    args []string
}
```

Populate this type within `Commands`, supply it as an Fx value, and require that value in the `run` function:

```go
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
    cliParams := &cliParams{
        GlobalParams: globalParams,
    }
    var useTLS bool
    cmd := cobra.Command{
        Use: "foo", ...,
        RunE: func(cmd *cobra.Command, args []string) error {
            cliParams.args = args
            return fxutil.OneShot(run,
                fx.Supply(cliParams),
                fx.Supply(core.CreateaBundleParams()),
                core.Bundle,
                ..., // any other bundles needed for this app
            )
        },
    }
	cmd.PersistentFlags().BoolVarP(&cliParams.useTLS, "usetls", "", "", "force TLS use")

    return []*cobra.Command{cmd}
}

func run(cliParams *cliParams, log log.Component) error {
    if (cliParams.Verbose) {
        log.Info("executing foo")
    }
    ...
}
```

This example includes cli params drawn from GlobalParams (`Verbose`), from subcommand-specific args (`useTLS`), and from Cobra (`args`).

### Daemon Apps

A daemon app is one that runs "forever", such as `agent run`.
Use the `fxutil.Run` helper function for this variety of app:

```go
cmd := cobra.Command{
    Use: "foo", ...,
    RunE: func(cmd *cobra.Command, args []string) error {
        return fxutil.Run(
            fx.Supply(core.BundleParams{}),
            core.Bundle,
            ..., // any other bundles needed for this app
            fx.Supply(foo.BundleParams{}),
            foo.Bundle, // the bundle implementing this app
        )
    },
}
```
