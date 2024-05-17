# Testing components

Testing is an essential part of the software development life cycle. This page covers everything you need to know about testing components.

One of the core benefits of using components is that each component isolates its internal logic behind its interface. Focus on asserting that each implementation behaves correctly.

To recap from the [previous page](creating-components.md), a component was created that compresses the payload before sending it to the Datadog backend. The component has two separate implementations.

This is the component's interface:

=== ":octicons-file-code-16: comp/compression/def/component.go"
    ```go
    type Component interface {
        // Compress compresses the input data.
        Compress([]byte) ([]byte, error)

        // Decompress decompresses the input data.
        Decompress([]byte) ([]byte, error)
    }
    ```

Ensure the `Compress` and `Decompress` functions behave correctly.

Writing tests for a component implementation follows the same rules as any other test in a Go project. See the [testing package](https://pkg.go.dev/testing) documentation for more information.

For this example, write a test file for the `zstd` implementation. Create a new file named `component_test.go` in the `impl-zstd folder`. Inside the test file, initialize the component's dependencies, create a new component instance, and test the behavior.

### Initialize the component's dependencies

All components expect a `Requires` struct with all the necessary dependencies. To ensure a component instance can be created, create a `requires` instance.

The `Requires` struct declares a dependency on the config component and the log component. The following code snippet shows how to create the `Require` struct:

=== ":octicons-file-code-16: comp/compression/impl-zstd/component_test.go"
    ```go
    package implzstd
    
    import (
      "testing"

      configmock "github.com/DataDog/datadog-agent/comp/core/config/mock"
      logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
    )
    
    func TestCompress(t *testing.T) {
      logComponent := configmock.New(t)
      configComponent := logmock.New(t)
      
      requires := Requires{
        Conf: configComponent,
        Log: logComponent,
      }
      // [...]
    }
    ```
    
To create the log and config component, use their respective mocks. The [mock package](creating-components.md#the-mock-folder) was mentioned previously in the [Creating a Component page](creating-components.md).
    

### Testing the component's interface

Now that the `Require` struct is created, an instance of the component can be created and its functionality tested:

=== ":octicons-file-code-16: comp/compression/impl-zstd/component_test.go"
    ```go
    package implzstd
    
    import (
      "testing"

      configmock "github.com/DataDog/datadog-agent/comp/core/config/mock"
      logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
    )
    
    func TestCompress(t *testing.T) {
      logComponent := configmock.New(t)
      configComponent := logmock.New(t)
      
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

Sometimes a component uses [Fx lifecycle](fx.md#lifecycle) to add hooks. It is a good practice to test the hooks as well. 

For this example, imagine a component wants to add some hooks into the app lifecycle. Some code is omitted for simplicity:

=== ":octicons-file-code-16: comp/somecomponent/impl/component.go"
    ```go
    package impl

    import (
      "context"
      
      somecomponent "github.com/DataDog/datadog-agent/comp/somecomponent/def"
      compdef "github.com/DataDog/datadog-agent/comp/def"
    )

    type Requires struct {
      Lc      compdef.Lifecycle
    }

    type Provides struct {
      Comp somecomponent.Component
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

The goal is to test that the component updates the `started` and `stopped` fields.

To accomplish this, create a new lifecycle instance, create a `Require` struct instance, initialize the component, and validate that calling `Start` on the lifecycle instance calls the component hook and executes the logic.

To create a lifecycle instance, use the helper function `compdef.NewTestLifecycle()`. The function returns a lifecycle wrapper that can be used to populate the `Requires` struct. The `Start` and `Stop` functions can also be called.

!!! Info 
    You can see the `NewTestLifecycle` function [here](https://github.com/DataDog/datadog-agent/blob/c9395595e34c6a96de9446083b8b1d0423bed991/comp/def/lifecycle_mock.go#L21)

=== ":octicons-file-code-16: comp/somecomponent/impl/component_test.go"
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
    
For this example, a type cast operation had to be performed because the `started` field is private. Depending on the component, this may not be necessary.
