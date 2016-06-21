This package is responsible of scanning different sources searching for Agent checks' configuration files.

Check configurations may be contained within files on disk, environment variables, external databases: for
each source, the Agent has a specific _Provider_ implementing the `ConfigProvider` interface.

Check configurations may come in different format, for example Yaml code in the case of config files on disk.
Every configuration, regardless of the format, must be unmarshalled into a `CheckConfig` struct.

Usage example:
```go
var configs []loader.CheckConfig
for _, provider := range configProviders {
  c, _ := provider.Collect()
  configs = append(configs, c...)
}
```
