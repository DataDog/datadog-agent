
# How to use `internal` folders

[Since Go 1.4](https://go.dev/doc/go1.4#internalpackages), Go supports the use of [`internal` folders](https://docs.google.com/document/d/1e8kOo3r51b2BWtTs_1uADIA5djfXhPT36s6eHVRIvaU/edit) to control the public API of a Go module: a package A can only be imported from packages whose path shares the prefix up until the last `internal` in A's path. For example, a package with path `a/b/internal/c/d` can only be imported by packages within the `a/b` folder. The compiler will enforce this and fail to build any code that breaks this rule.

This can be used to have some packages be internal to a given folder, which is useful for decoupling different parts of our codebase. 
When adding new code, carefully consider what API is exported, both by taking care of what symbols are uppercase and by making judicious use of `internal` folders.

Use the following guidelines in order to decide when and how to use `internal` folders:

1. When in doubt, prefer hiding public API: it's much easier to refactor code to expose something that was private than doing it the other way around.
2. When refactoring a struct into its own package, try moving it into its own `internal` folder if possible (see appendix below for an example).
3. If the folder you are editing already has `internal` folders, try to move code to these `internal` folders instead of creating new ones.
4. When creating new `internal` folders, try to use the deepest possible `internal` folder to limit the packages that can import yours. 
   For example, if making `a/b/c/d` internal, consider moving it to `a/b/c/internal/d` instead of `a/internal/b/c/d`.
   With the first path, code from `a/b` won't be able to access the `d` package, while it could with the second path.

## Appendix: refactor example

Sometimes one wants to hide private fields of a struct from other code in the same package to enforce a particular code invariant. In this case, the struct should be moved to a different folder within the same package **making this folder `internal`**.

Consider a module named `example.com` where you want to move `exampleStruct` from the `a` package to a subfolder to hide its private fields from `a`s code.

Before the refactor, the code will look like this:

```go
// a/code.go
package a

type exampleStruct struct {
    // some fields
}

func doSomethingWithExampleStruct(e exampleStruct) {
    // some code goes here ...
}
```

After the refactor, you should move `exampleStruct` to a `a/internal/b` folder:

```go
// a/internal/b/examplestruct.go
package b

type ExampleStruct struct {
    // some fields
}
```

and import this package from `a/code.go`:

```go
// a/code.go
package a

import (
    "example.com/a/internal/b"
)

func doSomethingWithExampleStruct(e b.exampleStruct) {
    // some code goes here
}
```

In this way, no new public API is exposed on the `a` folder: `ExampleStruct` remains private to `a`, while we have improved encapsulation as we wanted.
