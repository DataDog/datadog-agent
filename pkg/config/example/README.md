# Agent configuration examples

This directory holds a reference copy of the example configuration files
(`*.yaml.example`) shipped with each Agent flavor on each platform.

## Generated files: do not edit

Every `<build-type>_<os>.yaml.example` file (`datadog-agent_*`, `dca_*`,
`dcacf_*`, `dogstatsd_*`, `iot-agent_*`, `system-probe_*`) is rendered from the
Agent configuration schema in `pkg/config/schema/yaml/` by
`dda inv schema.template-all`.

**Do not edit these files by hand, and do not commit regenerated versions in
your PRs.** The packages built by the Agent pipeline render their example
configs from the schema at build time and do not read these files. The copies
here are refreshed automatically after each Agent release: the release
automation opens a `[release] Refresh config templates for <version>` PR with
the templates produced by the `generate_config_templates` job of that release's
pipeline. This way they always reflect what a released Agent ships.

To change the content of a generated example, edit the schema instead (see
`doc/how-to/agent-schema/settings.md`). The change will show up here after the
next release. If you run `dda inv schema.template-all` locally to check your
change, revert the files it rewrites in this directory before committing.

## Static files: edit by hand

The following files are not generated from the schema. They are packaged as-is
(see `pkg/config/BUILD.bazel`) and must be edited directly:

- `security-agent.yaml.example`
- `application_monitoring.yaml.example`
