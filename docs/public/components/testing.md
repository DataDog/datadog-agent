# Testing components

Testing is an essential part of the development process, and it’s a critical part of the software development life cycle. Throughout this page will cover everything you need to know about testing components.

One of the core benefits of using components is that each component isolate the internal logic behind its interface. We should focus on asserting that each implementatioin behaves correctly. 

To recapitulate form the [previous page](creating-components.md). We created a component that compresses the payload before sending it to the Datadog backend. The component had twop separate implementations.

This our components interface:

=== ":octicons-file-code-16: comp/compression/def/component.go"
    ```go
    type Component interface {
        // Compress compresses the input data.
        Compress([]byte) ([]byte, error)

        // Decompress decompresses the input data.
        Decompress([]byte) ([]byte, error)
    }
    ```

We must ensure the `Compress` and `Decompress` functions behave correctly. 

Writing test for a component implementation follow the same rules as any other test in a Go project. Here is the documentation for the [testing](https://pkg.go.dev/testing) package.

For this example we are going to write a test file for the `zstd` implementation. For that we are going to create a new file in our `impl-zstd` folder and we are going to name it `component_test.go`. Inside our test file we are going to initialize the component's dependencies, create a new component instance and test the behaviour.

### Initialize the component's dependencies

All components expect a `Requires` struct with all the necessary. Inside that struct we declare all our dependencies. To ensure we can create a component instance we are going to create a `requires` instance. 

Our `Requires` struct declared a dependency on the config component and the logs component. The next code snippet shows how to create the `Require` struct.

=== ":octicons-file-code-16: comp/compression/impl-zstd/component_test.go"
    ```go
    package implzstd
    
    import (
      "testing"

      "github.com/DataDog/datadog-agent/pkg/util/fxutil"

      "github.com/DataDog/datadog-agent/comp/core/config"
      "github.com/DataDog/datadog-agent/comp/core/log/logimpl"
    )
    
    func TestCompress(t *testing.T) {
      logComponent := fxutil.Test[log.Component](t, logimpl.MockModule())
      configComponent := fxutil.Test[config.Component](t, config.MockModule())
      
      requires := Requires{
        Conf: configComponent,
        Log: logComponent,
      }
      // [...]
    }
    ```
    
To create the log and config component from their respective mocks we used `fxutil.Test[T]` generic function. This functions make sure to initialize any Fx dependency needed for the Mock and return a instance of `T`.

!!! warning "is not intended to use `fxutil` package inside the implementation"
    We are migrating from a previous file structure to the one outlined in the [creating a component page](creating-components.md#file-hierarchy). For now it is ok to use as it allow us to migrate one component at a time. in the future there we would be restrciting the use Fx related code inside the implementation folder using a custom Go linter. 
    

### Testing

Now we have our `Require` struct we can create an instance of our component and test iots functionality.

=== ":octicons-file-code-16: comp/compression/impl-zstd/component_test.go"
    ```go
    package implzstd
    
    import (
      "testing"

      "github.com/DataDog/datadog-agent/pkg/util/fxutil"

      "github.com/DataDog/datadog-agent/comp/core/config"
      "github.com/DataDog/datadog-agent/comp/core/log/logimpl"
    )
    
    func TestCompress(t *testing.T) {
      logComponent := fxutil.Test[log.Component](t, logimpl.MockModule())
      configComponent := fxutil.Test[config.Component](t, config.MockModule())
      
      requires := Requires{
        Conf: configComponent,
        Log: logComponent,
      }
      
      provides := NewComponent(requires)
      component := provides.Comp
      
      result, err := component.Compress([]byte("Hello World"))
      assert.Nil(t, err)
  
      assert.Equal(t, ..., result)
    }
    ```

### Testing lifecycle hooks

There are going to be times in which our component uses [Fx lifecycle](fx.md#lifecycle) to add hooks. It is a good practice to test the hooks as well. 

For this example let's imagine our component wants to add some hooks into the app lifecycle. I will omit some code for simplicity

=== ":octicons-file-code-16: comp/someomponent/impl/component.go"
    ```go
    package impl

    import (
      "context"
      
      someomponent "github.com/DataDog/datadog-agent/comp/someomponent/def"
      compdef "github.com/DataDog/datadog-agent/comp/def"
    )

    type Requires struct {
      Lc      compdef.Lifecycle
    }

    type Provides struct {
      Comp someomponent.Component
    }

    type component struct {
      started  bool
      stopped bool
    }

    func (c *component) start() error {
      // [...]
      
      c.started = true

      return nil
    }

    func (h *healthprobe) stop() error {
      // [...]
      
      c.stopped = true
      c.started = false

      return nil
    }

    // NewComponent creates a new healthprobe component
    func NewComponent(reqs Requires) (Provides, error) {
      provides := Provides{}
      comp := &component{}

      reqs.Lc.Append(compdef.Hook{
        OnStart: func(ctx context.Context) error {
          return comp.start()
        },
        OnStop: func(ctx context.Context) error {
          return comp.stop()
        },
      })

      provides.Comp = comp
      return provides, nil
    }
    ```

We want to test that our component updates the `started` and `stopped` fields. 

To accomplish it we are going to create a new lifecycle instance, create a `Require` struct instance, intialize our component, and validate that calling `Start` on the lifecycle instace to validate that the component hook has being called and the logic has being execuited.

To create a lifecycle instance, we will use a helper function. `compdef.NewTestLifecycle()`. The function returns a lyfecycle wrapper that we can use to populate our `Requires` struct. Also, we can  call the `Start` and `Stop` functions.

!!! Info 
    <!-- TODO add link to NewTestLifecycle function once this PR https://github.com/DataDog/datadog-agent/pull/25184 is merged-->
    You can see the `NewTestLifecycle` function [here]()

=== ":octicons-file-code-16: comp/someomponent/impl/component.go"
    ```go
    package impl

    import (
      "context"
      "testing"

      compdef "github.com/DataDog/datadog-agent/comp/def"
      "github.com/stretchr/testify/assert"
    )

    func TestStartHook(t *testing.T) {
      lc := compdef.NewTestLifecycle()
      
      requires := Requires{
        Lc:  lc,
      }

      provides, err := NewComponent(requires)

      assert.NoError(t, err)

      assert.NotNil(t, provides.Comp)
      internalComponent := provides.Comp.(*component)

      ctx := context.Background()
      assert.NoError(t, lc.Start(ctx))

      assert.True(t, internalComponent.started)
    }
    ```
    
For our example we had to perform a type cast operation, because the `started` field is private. Dependeding our your component you might not have to do that.