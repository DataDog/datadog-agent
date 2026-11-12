# generatedsettings (committed snapshot)

This directory is a committed snapshot of the Go module
`github.com/DataDog/datadog-agent/pkg/config/setup/generatedsettings`. That
module registers every Agent setting declared in the config schema
(`pkg/config/schema/yaml/`).

This is used by outside repository relying on the Agent.

## Do not edit or refresh this snapshot

**Do not edit these files by hand, and do not commit a refreshed snapshot in
your PRs.** Release automation refreshes this directory after each Agent release
by running `dda inv schema.codegen-snapshot`.

In-repo builds never use this snapshot. `go.work`, every in-repo `replace`
directive, and Bazel point to `../generatedsettings_local`, which is regenerated
from the schema on every build. As a result, a schema change takes effect in the
repository right away. This snapshot exists only for external consumers of
`pkg/config/setup`, such as the OTel Collector. They download it at a released
tag and have no codegen step.

To change a setting, edit the schema (see
`doc/how-to/agent-schema/settings.md`). The snapshot picks up the change after
the next release.
