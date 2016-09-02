## package `loader`

This package is responsible of scanning different sources searching for Agent checks' configuration files, extract
configuration instances and provide the corresponding check objects.

### Configuration Providers
Providers implement the `ConfigProvider` interface and are responsible for scanning different sources like files on
disk, environment variables or databases, searching for check configurations. Every configuration, regardless of the
format, must specify at least one check `instance`. Providers dump every configuration they find into a `CheckConfig`
struct containing an array of configuration instances. Configuration instances are converted in YAML format so that a
check object will be eventually able to convert them into the appropriate data structure.

Usage example:
```go
var configs []loader.CheckConfig
for _, provider := range configProviders {
  c, _ := provider.Collect()
  configs = append(configs, c...)
}
```

### Check Loaders
Loaders implement the `CheckLoader` interface, they are responsible to instantiate one object of type `check.Check` for
every configuration instance within a `CheckConfig` object. A Loader usually invokes the `Configure` method on check
objects passing in the configuration instance in YAML format: how to use it, it's up to the check itself.

Usage example:
```go
// given a list of configurations, try to load corresponding checks using different loaders
checks := []check.Check{}
for _, conf := range configs {
  for _, loader := range loaders {
    res, err := loader.Load(conf)
    if err == nil {
      checks = append(checks, res...)
    }
  }
}
// `checks` contains one check per configuration instance found.
```

## package `scheduler`

This package is responsible of sending checks to the execution pipeline with an interval specified for any number
of `instance` configurations coming along with any check. Only one `Scheduler` instance is supposed to run at any time
and even if this is not a requirement, a use case for multiple schedulers didn't show up and therefore wasn't tested.

### Scheduler

A `Scheduler` instance keeps a collection of `time.Ticker`s associated to a list of `check.Check`s: every time the
ticker fires, all the checks in that list are sent to the execution pipeline. Every queue runs in its own goroutine.
The `Scheduler` expose an interface based on methods attached to the struct but the implementation makes use of
channels to synchronize the queues and to talk with the scheduler loop to send commands like `Run`, `Reload`, `Stop`.
