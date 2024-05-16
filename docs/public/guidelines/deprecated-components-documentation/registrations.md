# Component Registrations

Components generally need to talk to one another! In simple cases, that occurs by method calls. But in many cases, a single component needs to communicate with a number of other components that all share some characteristics. For example, the `comp/core/health` component monitors the health of many other components, and `comp/workloadmeta/scheduler` provides workload events to an arbitrary number of subscribers.

The convention in the Agent codebase is to use [value groups](../../components/fx.md#value-groups) to accomplish this. The _collecting_ component requires a slice of some _collected type_, and the _providing_ components provide values of that type. Consider an example case of an HTTP server component to which endpoints can be attached. The server is the collecting component, requiring a slice of type `[]*endpoint`, where `*endpoint` is the collected type. Providing components provide values of type `*endpoint`.

The convention is to "wrap" the collected type in a `Registration` struct type which embeds `fx.Out` and has tag `group:"pkgname"`, where `pkgname` is the short package name (Fx requires a group name, and this is as good as any). This helps providing components avoid the common mistake of omitting the tag. Because it is wrapped in an exported `Registration` type, the collected type can be an unexported type, as in the example below.

The collecting component should define the registration type and a constructor for it:

=== ":octicons-file-code-16: comp/server/component.go"
    ```go
    // ...
    // Server endpoints are provided by other components, by providing a server.Registration
    // instance.
    // ...
    package server

    type endpoint struct {  // (the collected type)
        ...
    }

    type Registration struct {
        fx.Out

        Endpoint endpoint `group:"server"`
    }

    // NewRegistration creates a new Registration instance for the given endpoint.
    func NewRegistration(route string, handler func()) Registration { ... }
    ```

Its implementation then requires a slice of the collected type (`endpoint`), again using `group:"server"`:

=== ":octicons-file-code-16: comp/server/server.go"
    ```go
    // endpoint defines an endpoint on this server.
    type endpoint struct { ... }

    type dependencies struct {
        fx.In

        Registrations []endpoint `group:"server"`
    }

    func newServer(deps dependencies) Component {
        // ...
        for _, e := range deps.Registrations {
            if e.handler == nil {
                continue
            }
            // ...
        }
        // ...
    }
    ```

It's good practice to ignore zero values, as that allows providing components to skip the registration if desired.

Finally, the providing component (in this case, `foo`) includes a registration in its output as an additional provided type, beyond its `Component` type:

=== ":octicons-file-code-16: comp/foo/foo.go"
    ```go
    func newFoo(deps dependencies) (Component, server.Registration) {
        // ...
        return foo, server.NewRegistration("/things/foo", foo.handler)
    }
    ```

This technique has some caveats to be aware of:

* The providing components are instantiated before the collecting component.
* Fx treats value groups as the collecting component depending on all of the providing components. This means that the providing components cannot depend on the collecting component, as this would represent a dependency cycle.
* Fx will instantiate _all_ declared providing components before the collecting component, regardless of whether their `Component` type is required. This may lead to components being instantiated in unexpected circumstances.
