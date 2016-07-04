This package is responsible of scanning different sources searching for Agent checks' configuration files. Check
configurations consist in one or more `instance`s, every loader must be capable to break down such `instance`s
into distinct elements.

Check configurations may be contained within files on disk, environment variables, external databases: for
each source, the Agent has a specific _Provider_ implementing the `ConfigProvider` interface.

Check configurations may come in different format, for example Yaml code in the case of config files on disk, or
environment variables following certain conventions for their names.

Every configuration, regardless of the format, must specify at least one check `instance`: each instance will be
translated to Yaml code, the common language to carry around instance configurations within the Agent code. A
 `CheckConfig` struct contains a list of Yaml configuration text elements, one per instance.

Usage example:
```go
var configs []loader.CheckConfig
for _, provider := range configProviders {
  c, _ := provider.Collect()
  configs = append(configs, c...)
}
```
