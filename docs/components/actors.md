# Actors

Many components use the [actor model](https://en.wikipedia.org/wiki/Actor_model) to structure their operations.
Briefly, this is a "main loop" that reacts to events one-by-one, updating its internal state.
This effectively serializes execution, removing the need for concurrency primitives and simplifying implementation.

The `pkg/util/actor` package provides support for this model.
It handles the particulars of starting and stopping an actor goroutine, and supports liveness monitoring.
Use it like this:

```go
type listener struct {
    // events are the things to which this component reacts
    events chan event

    // actor encapsulates this agent's "run loop"
    actor Actor
}

func newTestComp(lc fx.Lifecycle) (*listener) {
    c := &listener{
        events: ...,
        actor: actor.New(lc, c.run),
    }
    return c
}

func (c *listener) run(ctx context.Context, alive <-chan struct{}) {
    for {					// the main loop
        select {
        case <-c.events:
            ...             // handle the event
        case <-alive: 		// consuming an alive message indicates the loop is healthy
        case <-ctx.Done(): 	// actor should stop when ctx is complete
            return
        }
    }
}
```
