## package `health`

The `health` package handles internal healthchecks for the agents, that allow to
check every asynchronous component is running as intended.

For more information on the context, see the `agent-healthcheck.md` proposal.

### How to add a component?

- First, you need to register, by calling `health.Register` with a user-visible name. You will
receive a `*health.Handle` to keep. As soon as `Register` is called, you need to start reading
the channel to be considered healthy.

- If you want to register a component for the `Startup` probe, you need to call `RegisterStartup()`.
This is useful for components that need to perform some initialization before being considered healthy.
You will need to first register the component, then do the initialization, and finally read from the channel.

- In your main goroutine, you need to read from the `handle.C` channel, at least every 15 seconds.
This is accomplished by using a `select` statement in your main goroutine. If the channel is full
(after two tries), your component will be considered unhealthy, which might result in the agent
getting killed by the system.

- If your component is stopping, it should call `handle.Deregister()` before stopping. It will
then be removed from the healthcheck system.

### Where should I tick?

It depends on your component lifecycle, but the check's purpose is to check that your component
is able to process new input and act accordingly. For components that read input from a channel,
you should read the channel from this logic.

This is usually highly unlikely, but it's exactly the scope of this system: be able to
detect if a component is frozen because of a bug / race condition. This is usually the only
kind of issue that could be solved by the agent restarting.
