# Common patterns

TODO: A page documenting common pattern:

* Groups
* Optional
* Enable/Disable state
* ...

## Groups

`Fx` [groups](https://pkg.go.dev/go.uber.org/fx#hdr-Value_Groups) are a useful feature that make it easier to produce
and consume many values of the same type produced in different parts of the codebase. A component can add any type into
groups which can be consumed by other components.

For example:

Here, two components add a `server.Endpoint` type to the `server` group.

=== ":octicons-file-code-16: todolist/todolist.go"
    ```go
    type Provides struct {
        comp     Component
        Endpoint server.Endpoint `group:"server"`
    }
    ```

=== ":octicons-file-code-16: users/users.go"
    ```go
    type Provides struct {
        comp     Component
        Endpoint server.Endpoint `group:"server"`
    }
    ```

Here, a component requests all the types added to the `server` group. This takes the form of a slice received at
instantiation (note once again the `group` label but in `fx.In` struct).

=== ":octicons-file-code-16: server/server.go"
    ```go
    type Requires struct {
        Endpoints []Endpoint `group:"server"`
    }
    ```

