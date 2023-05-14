# Atomic Access

tl;dr: use `go.uber.org/atomic` for all atomic access.  Use pointers (`*atomic.Uint64`, etc.) in structs to ensure proper alignment.
Expvars are accessed atomically and must also be used via pointers in structs.

## Prefer Not

First, atomics are a _very_ low-level concept and full of subtle gotchas and dramatic performance differences across platforms.
If you can use something higher-level, such as something from the `sync` package, prefer to do so.
Otherwise, if you are in search of performance, be sure to benchmark your work carefully and on multiple platforms.

It's tempting to use an atomic to avoid complaints from the race detector, but this is almost always a mistake -- it is merely hiding your race from the race detector.
Consider carefully why the implementation is racing, and try to address that behavior directly.

The exception to this is tests, where atomics can be useful for sensing some value that you would like to assert on that is manipulated in another goroutine.
Even here, be wary of race conditions, such as assuming that a background goroutine has executed before the test goroutine makes its assertions.

## Use uber-go/atomic

OK, so you've decided to use atomics.
Use `go.uber.org/atomic`, rather than the built-in `sync/atomic` package.

### How

Always declare atomic types using a pointer.
This ensures proper alignment.

```golang
//  global variable
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

Use the `atomic.Uint64` methods to perform atomic operations on the value.
These include some conveniences not available in `sync/atomic`, such as Inc/Dec and `atomic.Bool`.

If the additional pointer allocation poses an undue performance burden, include the value as the *first* element in the struct (to ensure alignment) and include a comment indicating:
 * that it must remain in that position; and
 * why a pointer was not suitable.

Pointers to atomic types marshal correctly to JSON as their enclosed value. Unmarshaling does the reverse, except that missing values are represented as nil, rather than an atomic type with zero value.

### Why

There are two main issues with the built-in `sync/atomic` package:

1. It has an unresolved [alignment bug](https://pkg.go.dev/sync/atomic#pkg-note-BUG) requiring users to manually ensure alignment.
   This is frequently forgotten, and only causes issues on less-common platforms, leading to undetected bugs.

1. It is very easy to access a raw integer variable using a mix of atomic and non-atomic operations.
   This mix may be enough to satisfy the race detector, but not sufficient to actually prevent undefined behavior.

## Expvars Too

Types such as `expvar.Int` are simple wrappers around an integer, and are accessed using `sync/atomic`.
That makes them susceptible to the [alignment issues](https://pkg.go.dev/sync/atomic#pkg-note-BUG) described above.
Go will properly align variables (whether global or local) but not struct fields, so any expvar types embedded in a struct must use a pointer:

```go
type SomeStuff struct {
    good *expvar.Int{}
    bad expvar.Int{} // don't do this!
}
```
