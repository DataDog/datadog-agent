# Actors

Many components use the [actor model](https://en.wikipedia.org/wiki/Actor_model) to structure their operations.
Briefly, this is a "main loop" that reacts to events one-by-one, updating its internal state.
This effectively serializes execution, removing the need for concurrency primitives and simplifying implementation.

The `pkg/util/actor` package provides support for this model.
It handles the particulars of starting and stopping an actor goroutine, and supports liveness monitoring.
Use it like this:

```go
type listener struct {
    actor Actor
}

func newTestComp(lc fx.Lifecycle) (*listener, health.Registration) {
    c := &listener{}
    c.actor.HookLifecycle(lc, c.run) // hook the actor into the Fx lifecycle
    return c, reg
}

func (c *listener) run(ctx context.Context, alive <-chan struct{}) {
    for {					// the main loop
        select {
        case <-alive: 		// consuming the message indicates the loop is healthy
        case <-ctx.Done(): 	// actor stops when the context is complete
            return
        }
    }
}
```
