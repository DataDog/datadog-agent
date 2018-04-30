## package `providers`

Providers implement the `ConfigProvider` interface and are responsible for scanning different sources like files on
disk, environment variables or databases, searching for integration configurations. Every configuration, regardless of the
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