## package `check`

This package is responsible of defining the types representing an agent check along with
an interface implemented by the code responsible to create a check instance based on an
existing configuration.

### Check Loaders
Loaders implement the `CheckLoader` interface, they are responsible to instantiate one object of type `check.Check` for
every configuration instance within a `integration.Config` object. A Loader usually invokes the `Configure` method on check
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
