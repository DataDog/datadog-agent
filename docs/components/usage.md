# Using Components and Bundles

## Component Dependencies

Component dependencies are automatically determined from the arguments to a component constructor.
Most components have a few dependencies, and use a struct named `dependencies` to represent them:

```go
type dependencies struct {
    fx.In

    Lc fx.Lifecycle
    Params internal.BundleParams
    Config config.Module
    Log log.Module
    // ...
}

func newThing(deps dependencies) Component {
    t := &thing{
        log: deps.Log,
        ...
    }
    deps.Lc.Append(fx.Hook{OnStart: t.start})
    return t
}
```

## Testing

Testing for a component should use `fxtest` to create the component.
This focuses testing on the API surface of the component against which other components will be built.
Per-function unit tests are, of course, also great where appropriate!

Here's an example testing a component with a mocked dependency on `other`:

```go
func TestMyComponent(t *testing.T) {
    var comp Component
    var other other.Component
    app := fxtest.New(t,
        Module,              // use the real version of this component
        other.MockModule(),    // use the mock version of other
        fx.Populate(&comp),  // get the instance of this component
        fx.Populate(&other), // get the (mock) instance of the other component
    )

    // start and, at completion of the test, stop the components
    defer app.RequireStart().RequireStop()

    // cast `other` to its mock interface to call mock-specific methods on it
    other.(other.Mock).SetSomeValue(10)                      // Arrange
    comp.DoTheThing()                                        // Act
    require.Equal(t, 20, other.(other.Mock).GetSomeResult()) // Assert
}
```

If the component has a mock implementation, it is a good idea to test that mock implementation as well.
