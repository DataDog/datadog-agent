<!-- # Common patterns

TODO: A page documenting common pattern:

* Groups
* Optional
* Enable/Disable state
* ... -->

## Groups

Fx [groups](https://pkg.go.dev/go.uber.org/fx#hdr-Value_Groups) help you produce and group together values of the same type, even if these values are produced in different parts of the codebase. A component can add any type into a group; this group can then consumed by other components.

In the following example, a component add a `server.Endpoint` type to the `server` group.

=== ":octicons-file-code-16: comp/users/users.go"
    ```go
    type Provides struct {
        comp     Component
        Endpoint server.Endpoint `group:"server"`
    }
    ```

In the following example, a component requests all the types added to the `server` group. This takes the form of a slice received at
instantiation.

=== ":octicons-file-code-16: comp/server/server.go"
    ```go
    type Requires struct {
        Endpoints []Endpoint `group:"server"`
    }
    ```
