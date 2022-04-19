# Atomic Access

tl;dr: use `github.com/uber-go/atomic` for all atomic access.  Use pointers (`*atomic.Uint64`, etc.) in structs to ensure proper alignment.

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
Use `github.com/uber-go/atomic`, rather than the built-in `sync/atomic` package.

### How

If your atomic is a global variable, you may declare it using its zero value:

```golang
var maxFooCount atomic.Uint64
```

If your atomic is in a struct, declare it as a pointer, to ensure proper alignment:

```golang
type FooTracker struct {
    maxCount *atomic.Uint64
}
```

In this case, initialize the pointer in the constructor (`atomic.NewUint64(..)`) and use the `atomic.Uint64` methods to perform atomic operations on the value.

If the additional pointer allocation poses an undue performance burden, include the value as the first element in the struct and include a comment indicating
 * that it must remain in that position; and
 * why a pointer was not suitable.

### Why

There are two main issues with the built-in `sync/atomic` package:

1. It has an unresolved [alignment bug](https://pkg.go.dev/sync/atomic#pkg-note-BUG) requiring users to manually ensure alignment.
   This is frequently forgotten, and only causes issues on less-common platforms, leading to undetected bugs.

1. It is very easy to access a raw integer variable using a mix of atomic and non-atomic operations.
   This mix may be enough to satisfy the race detector, but not sufficient to actually prevent undefined behavior.
