
<!-- !!! warning "TODO: rework this entire page to include:"

    * Basic info about fx and dependency injection
    * Provide, Supply and Invoke function
    * fx App
    * value groups
    * Lifecycle (TODO decide how we want to offer lifecycle within depending on FX)
    * ... -->

# Overview of Fx

The Agent uses [Fx](https://uber-go.github.io/fx) as its application framework. While the linked Fx documentation is thorough, it can be a bit difficult to get started with. This document describes how Fx is used within the Agent in a more approachable style.

## What Is It?

Fx's core functionality is to create instances of required types "automatically," also known as [dependency injection](https://en.wikipedia.org/wiki/Dependency_injection). Within the agent, these instances are components, so Fx connects components to one another. Fx creates a single instance of each component, on demand.

This means that each component declares a few things about itself to Fx, including the other components it depends on. An "app" then declares the components it contains to Fx, and instructs Fx to start up the whole assembly.

## Providing and Requiring

Fx connects components using types. Within the Agent, these are typically interfaces named `Component`. For example, `scrubber.Component` might be an interface defining functionality for scrubbing passwords from data structures:

=== ":octicons-file-code-16: scrubber/component.go"
    ```go
    type Component interface {
        ScrubString(string) string
    }
    ```

Fx needs to know how to *provide* an instance of this type when needed, and there are a few ways:

* [`fx.Provide(NewScrubber)`](https://pkg.go.dev/go.uber.org/fx#Provide) where `NewScrubber` is a constructor that returns a `scrubber.Component`. This indicates that if and when a `scrubber.Component` is required, Fx should call `NewScrubber`. It will call `NewScrubber` only once, using the same value everywhere it is required.
* [`fx.Supply(scrubber)`](https://pkg.go.dev/go.uber.org/fx#Supply) where `scrubber` implements the `scrubber.Component` interface. When another component requires a `scrubber.Component`, this is the instance it will get.

The first form is much more common, as most components have constructors that do interesting things at runtime. A constructor can return multiple arguments, in which case the constructor is called if _any_ of those argument types are required. Constructors can also return `error` as the final return type. Fx will treat an error as fatal to app startup.

Fx also needs to know when an instance is *required*, and this is where the magic happens. In specific circumstances, it uses reflection to examine the argument list of functions, and creates instances of each argument's type. Those circumstances are:

* Constructors used with `fx.Provide`. Imagine `NewScrubber` depends on the config module to configure secret matchers:
      ```go
      func NewScrubber(config config.Component) Component {
          return &scrubber{
              matchers: makeMatchersFromConfig(config),
          }
      }
      ```
* Functions passed to [`fx.Invoke`](https://pkg.go.dev/go.uber.org/fx#Invoke):
    ```go
    fx.Invoke(func(sc scrubber.Component) {
        fmt.Printf("scrubbed: %s", sc.ScrubString(somevalue))
    })
    ```
    Like constructors, Invoked functions can take multiple arguments, and can optionally return an error. Invoked functions are called automatically when an app is created.
* Pointers passed to [`fx.Populate`](https://pkg.go.dev/go.uber.org/fx#Populate).
   ```go
   var sc scrubber.Component
   // ...
   fx.Populate(&sc)
   ```
   Populate is useful in tests to fill an existing variable with a provided value. It's equivalent to `fx.Invoke(func(tmp scrubber.Component) { *sc = tmp })`.

    Functions can take multple arguments of different types, requiring all of them.

## Apps and Options

You may have noticed that all of the `fx` methods defined so far return an `fx.Option`. They don't actually do anything on their own. Instead, Fx uses the [functional options pattern](https://commandcenter.blogspot.com/2014/01/self-referential-functions-and-design.html) from Rob Pike. The idea is that a function takes a variable number of options, each of which has a different effect on the result.

In Fx's case, the function taking the options is [`fx.New`](https://pkg.go.dev/go.uber.org/fx#New), which creates a new [`fx.App`](https://pkg.go.dev/go.uber.org/fx#New). It's within the context of an app that requirements are met, constructors are called, and so on.

Tying the example above together, a very simple app might look like this:

```go
someValue = "my password is hunter2"
app := fx.New(
    fx.Provide(scrubber.NewScrubber),
    fx.Invoke(func(sc scrubber.Component) {
        fmt.Printf("scrubbed: %s", sc.ScrubString(somevalue))
    }))
app.Run()
// Output: scrubbed: my password is *******
```

For anything more complex, it's not practical to call `fx.Provide` for every component in a single source file. Fx has two abstraction mechanisms that allow combining lots of options into one app:

* [`fx.Options`](https://pkg.go.dev/go.uber.org/fx#Options) simply bundles several Option values into a single Option that can be placed in a variable. As the example in the Fx documentation shows, this is useful to gather the options related to a single Go package, which might include un-exported items, into a single value typically named `Module`.
* [`fx.Module`](https://pkg.go.dev/go.uber.org/fx#Module) is very similar, with two additional features. First, it requires a module name which is used in some Fx logging and can help with debugging. Second, it creates a scope for the effects of [`fx.Decorate`](https://pkg.go.dev/go.uber.org/fx#Decorate) and [`fx.Replace`](https://pkg.go.dev/go.uber.org/fx#Replace). The second feature is not used in the Agent.

So a slightly more complex version of the example might be:

=== ":octicons-file-code-16: scrubber/component.go"
    ```go
    func Module() fxutil.Module {
        return fx.Module("scrubber",
        fx.Provide(newScrubber))    // now newScrubber need not be exported
    }
    ```

=== ":octicons-file-code-16: main.go"
    ```go
    someValue = "my password is hunter2"
    app := fx.New(
        scrubber.Module(),
        fx.Invoke(func(sc scrubber.Component) {
            fmt.Printf("scrubbed: %s", sc.ScrubString(somevalue))
        }))
    app.Run()
    // Output: scrubbed: my password is *******
    ```

## Lifecycle

Fx provides an [`fx.Lifecycle`](https://pkg.go.dev/go.uber.org/fx#Lifecycle) component that allows hooking into application start-up and shut-down. Use it in your component's constructor like this:

```go
func newScrubber(lc fx.Lifecycle) Component {
    sc := &scrubber{..}
    lc.Append(fx.Hook{OnStart: sc.start, OnStop: sc.stop})
    return sc
}

func (sc *scrubber) start(ctx context.Context) error { .. }
func (sc *scrubber) stop(ctx context.Context) error { .. }
```

This separates the application's lifecycle into a few distinct phases:

* Initialization - calling constructors to satisfy requirements, and calling invoked functions that require them.
* Startup - calling components' OnStart hooks (in the same order the components were initialized)
* Runtime - steady state
* Shutdown - calling components' OnStop hooks (reverse of the startup order)

## Ins and Outs

Fx provides some convenience types to help build constructors that require or provide lots of types: [`fx.In`](https://pkg.go.dev/go.uber.org/fx#In) and [`fx.Out`](https://pkg.go.dev/go.uber.org/fx#Out). Both types are embedded in structs, which can then be used as argument and return types for constructors, respectively. By convention, these are named `dependencies` and `provides` in Agent code:

```go
type dependencies struct {
    fx.In

    Config config.Component
    Log log.Component
    Status status.Component
)

type provides struct {
    fx.Out

    Component
    // ... (we'll see why this is useful below)
}

func newScrubber(deps dependencies) (provides, error) { // can return an fx.Out struct and other types, such as error
    // ..
    return provides {
        Component: scrubber,
        // ..
    }, nil
}
```

In and Out provide a nice way to summarize and document requirements and provided types, and also allow annotations via Go struct tags. Note that annotations are also possible with [`fx.Annotate`](https://pkg.go.dev/go.uber.org/fx#Annotate), but it is much less readable and its use is discouraged.

### Value Groups

[Value groups](https://pkg.go.dev/go.uber.org/fx#hdr-Value_Groups) make it easier to produce and consume many values of the same type. A component can add any type into groups which can be consumed by other components.

For example:

Here, two components add a `server.Endpoint` type to the `server` group (note the `group` label in the `fx.Out` struct).

=== ":octicons-file-code-16: todolist/todolist.go"
    ```go
    type provides struct {
        fx.Out
        Component
        Endpoint server.Endpoint `group:"server"`
    }
    ```

=== ":octicons-file-code-16: users/users.go"
    ```go
    type provides struct {
        fx.Out
        Component
        Endpoint server.Endpoint `group:"server"`
    }
    ```

Here, a component requests all the types added to the `server` group. This takes the form of a slice received at
instantiation (note once again the `group` label but in `fx.In` struct).

=== ":octicons-file-code-16: server/server.go"
    ```go
    type dependencies struct {
        fx.In
        Endpoints []Endpoint `group:"server"`
    }
    ```

# Day-to-Day Usage

Day-to-day, the Agent's use of Fx is fairly formulaic. Following the [component guidelines](creating-components.md), or just copying from other components, should be enough to make things work without a deep understanding of Fx's functionality.
