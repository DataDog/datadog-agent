## package `scheduler`

This package is responsible of sending checks to the execution pipeline with an interval specified for any number
of `instance` configurations coming along with any check. Only one `Scheduler` instance is supposed to run at any time
and even if this is not a requirement, a use case for multiple schedulers didn't show up and therefore wasn't tested.

### Scheduler

A `Scheduler` instance keeps a collection of `time.Ticker`s associated to a list of `check.Check`s: every time the
ticker fires, all the checks in that list are sent to the execution pipeline. Every queue runs in its own goroutine.
The `Scheduler` expose an interface based on methods attached to the struct but the implementation makes use of
channels to synchronize the queues and to talk with the scheduler loop to send commands like `Run` and `Stop`.

Once a scheduler is stopped, restarting it with `Run` is not expected to work. A new one should be instantiated and
`Run` instead.
