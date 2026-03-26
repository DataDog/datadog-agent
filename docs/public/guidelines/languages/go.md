# Go guidelines

-----

## Imports

The imports defined in the `imports ( ... )` block of each Go file should be separated into the following sections, in order.

1. Standard library packages (e.g. `fmt`, `net/http`)
1. External packages (e.g. `github.com/stretchr/testify/assert`, `github.com/DataDog/datadog-agent/pkg/util/log`)
1. [Internal](#public-apis) packages (e.g. `github.com/DataDog/datadog-agent/<parent>/internal`)

This is not verified by our [static analysis](../../how-to/test/static-analysis.md) during CI. Instead, we suggest configuring your editor to keep imports properly sorted.

/// details | Editor setup
The [goimports](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) tool supports a "local packages" section. Use the flag `-local github.com/DataDog/datadog-agent`.

//// tab | VS Code / Cursor
See the [wiki](https://github.com/golang/vscode-go/wiki/features#format-and-organize-imports) for more details.

```json
{
  "gopls": {
    "formatting.local": "github.com/DataDog/datadog-agent"
  }
}
```
////

//// tab | Vim
Configure [vim-go](https://github.com/fatih/vim-go) as follows.

```vim
let g:go_fmt_options = {
\ 'goimports': '-local github.com/DataDog/datadog-agent',
\ }
```
////
///

## Public APIs

Go [supports](https://go.dev/doc/modules/layout#package-or-command-with-supporting-packages) the use of private `internal` packages to control the public API of a Go module. This prevents packages from being used outside of the parent module, which is useful for decoupling different parts of our codebase.

When adding new code, carefully consider what API is exported by both taking care of what symbols are uppercase and by making judicious use of `internal` directories.

1. When in doubt, prefer hiding public APIs as it's much easier to refactor code to expose something that was private than doing it the other way around.
1. When refactoring a struct into its own package, try moving it into its own `internal` directory if possible (see example below).
1. If the directory you are editing already has `internal` directories, try to move code to these `internal` directories instead of creating new ones.
1. When creating new `internal` directories, try to use the deepest possible `internal` directory to limit the packages that can import yours. For example, if making `a/b/c/d` internal, consider moving it to `a/b/c/internal/d` instead of `a/internal/b/c/d`. With the first path, code from `a/b` won't be able to access the `d` package, while it could with the second path.

/// details | Example
Sometimes one wants to hide private fields of a struct from other code in the same package to enforce a particular code invariant. In this case, the struct should be moved to a different folder within the same package **making this folder `internal`**.

Consider a module named `example` where you want to move `exampleStruct` from the `a` package to a subfolder to hide its private fields from `a`'s code.

Before the refactor, the code will look like this:

//// tab | :octicons-file-code-16: example/a/code.go
```go
package a

type exampleStruct struct {
    // Public
    Foo string
    // Private
    bar string
}

func doSomethingWithExampleStruct(e exampleStruct) {
    // some code goes here ...
}
```
////

After the refactor, you should move `exampleStruct` to an `a/internal/b` directory:

//// tab | :octicons-file-code-16: example/a/internal/b/examplestruct.go
```go
package b

type ExampleStruct struct {
    // Public
    Foo string
    // Private
    bar string
}
```
////

and import this package from `a/code.go`:

//// tab | :octicons-file-code-16: example/a/code.go
```go
package a

import (
    "example/a/internal/b"
)

func doSomethingWithExampleStruct(e b.ExampleStruct) {
    // some code goes here
}
```
////

In this way, no new public API is exposed on the `a` folder: `ExampleStruct` remains private to `a`, while we have improved encapsulation as we wanted.
///

## Atomics

/// details | Avoid atomics!
    type: warning

Atomics are a _very_ low-level concept and full of subtle gotchas and dramatic performance differences across platforms. If you can use something higher-level, such as something from the standard library's `sync` [package](https://pkg.go.dev/sync), prefer to do so. Otherwise, if you are in search of performance, be sure to benchmark your work carefully and on multiple platforms.

It's tempting to use an atomic to avoid complaints from the [race detector](../../how-to/test/unit.md#race-detection), but this is almost always a mistake -- it is merely hiding your race from the race detector. Consider carefully why the implementation is racing, and try to address that behavior directly.

The exception to this is tests, where atomics can be useful for sensing some value that you would like to assert on that is manipulated in another goroutine. Even here, be wary of race conditions, such as assuming that a background goroutine has executed before the test goroutine makes its assertions.
///

Always ensure that you:

1. Use [`go.uber.org/atomic`](https://pkg.go.dev/go.uber.org/atomic) instead of `sync/atomic`.

    /// details | Why?
    There are two main issues with the standard library's `sync/atomic` package.

    1. It has an [alignment bug](https://pkg.go.dev/sync/atomic#pkg-note-BUG) requiring users to manually ensure alignment. This is frequently forgotten, and only causes issues on less-common platforms, leading to undetected bugs.
    1. It is very easy to access a raw integer variable using a mix of atomic and non-atomic operations. This mix may be enough to satisfy the race detector, but not sufficient to actually prevent undefined behavior.
    ///

1. Declare atomic types using a pointer to ensure proper alignment.

```go
// global variable
var maxFooCount = atomic.NewUint64(42)

// in a struct
type FooTracker struct {
    maxCount *atomic.Uint64
}

func NewFooTracker() *FooTracker {
    return &FooTracker {
        maxCount: atomic.NewUint64(42),
    }
}
```

Use the `atomic.Uint64` methods to perform atomic operations on the value. These include some conveniences not available in `sync/atomic`, such as `Inc`/`Dec` and `atomic.Bool`.

If the additional pointer allocation poses an undue performance burden, do **both** of the following.

1. Include the value as the *first* element in the struct (to ensure alignment).
1. Add a comment indicating that it must remain in that position and why a pointer was not suitable.

Pointers to atomic types marshal correctly to JSON as their enclosed value. Unmarshaling does the reverse, except that missing values are represented as `nil`, rather than an atomic type with zero value.

Types such as `expvar.Int` are simple wrappers around an integer, and are accessed using `sync/atomic`. Go will properly align variables (whether global or local) but not struct fields, so any expvar types embedded in a struct must use a pointer.

<div class="grid cards" markdown>

-   :white_check_mark: Good

    ---

    ```go
    type Example struct {
        field *expvar.Int{}
    }
    ```

-   :x: Bad

    ---

    ```go
    type Example struct {
        field expvar.Int{}
    }
    ```

</div>

## Testing

### Failing fast

The functions in [`github.com/stretchr/testify/require`](https://pkg.go.dev/github.com/stretchr/testify/require) automatically abort the test when an assertion fails, whereas [`github.com/stretchr/testify/assert`](https://pkg.go.dev/github.com/stretchr/testify/assert) does not.

For example, given an error, `assert.NoError(t, err)` causes the test to be marked as a failure, but continues to the next statement, possibly leading to a `nil` dereference or other such failure. In contrast, `require.NoError(t, err)` aborts the test when an error is encountered.

Where a test makes a sequence of independent assertions, `assert` is a good choice. When each assertion depends on the previous having been successful, use `require`.

### Time

Tests based on time are a major source of flakes. If you find yourself thinking something like "the ticker should run three times in 500ms", you will be disappointed at how often that is not true in CI. Even if that test is not flaky, it will take at least 500ms to run. Summing such delays over thousands of tests means _very_ long test runs and slower work for everyone.

When the code you are testing requires time, the first strategy is to remove that requirement. For example, if you are testing the functionality of a poller, factor the code such that the tests can call the `poll()` method directly, instead of waiting for a Ticker to do so.

Where this is not possible, refactor the code to use a Clock from [`github.com/benbjohnson/clock`](https://pkg.go.dev/github.com/benbjohnson/clock). In production, create a `clock.Clock`, and in tests, inject a `clock.Mock`. When time should pass in your test execution, call `clock.Add(..)` to deterministically advance the clock.

A common pattern for objects that embed a timer is as follows:

```go
func NewThing(arg1, arg2) *Thing {
    return newThingWithClock(arg1, arg2, clock.New())
}

func newThingWithClock(arg1, arg2, clock clock.Clock) *Thing {
    return &Thing{
        ...,
        clock: clock,
    }
}

func TestThingFunctionality(t *testing.T) {
    clk := clock.NewMock()
    thing := newThingWithClock(..., clk)

    // ...

    clk.Add(100 * time.Millisecond)

    // ...
}
```

## Logging

Logging utilizes the [`log/slog`](https://pkg.go.dev/log/slog) package as its underlying framework.
You can access logging through `pkg/util/log` and the `comp/core/log` component wrappers.
Using the component wrapper is recommended, as it adheres to [component best practices](https://datadoghq.dev/datadog-agent/components/overview/).
