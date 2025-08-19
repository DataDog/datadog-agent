# Agent Configuration

This doc describes how to define new configuration parameters for the Agent.

1. Define your config.
2. Add it to the config template (optional).
3. Use your config in your code.
4. Request a review from the Agent configuration team ()

If you have any questions, head over to #agent-configuration and ask (datadog
internal).

## 1. Define Your Config

A config must be declared before it can be used. If you don't do this, the Agent
will log warnings about missing config at runtime that look like:

```
WARN | config keyconfig key "config.subsystem.bananas" is unknown
```

There are multiple places a config can be defined:

* Above all else, **prefer consistency**. If there's existing similar config,
  put the new config item alongside that existing config.

* If you want your config to be defined by the user in `system-probe.yml` then
  your declaration belongs in [`system_probe.go`].

* Otherwise it lives in the default `datadog-agent.yaml` file and goes in
  [`config.go`].


[`config.go`]: https://github.com/DataDog/datadog-agent/blob/main/pkg/config/setup/config.go
[`system_probe.go`]: https://github.com/DataDog/datadog-agent/blob/main/pkg/config/setup/system_probe.go

## 2. Add to Template

By default newly declared configs are not added to the sample config file a user
sees.

If you want your config to appear in the sample config file, add it to the
[config template].


[config template]: https://github.com/DataDog/datadog-agent/blob/main/pkg/config/config_template.yaml


## 3. Use Your Config

You can access your configured value (or the declared default) using the config
`model.Reader`. For example:

```go
if cfg.GetBool("config.subsystem.bananas") {
	// Go bananas
}
```

See the [package documentation] for available methods.


[package documentation]: https://pkg.go.dev/github.com/DataDog/datadog-agent/pkg/config/model#Reader


## 4. Request a Review!

Please add this label to your PRs: `team/agent-congfiguration`

This will summon a config wizard who can review your changes and suggest any
changes.
