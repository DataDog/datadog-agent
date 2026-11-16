# generatedsettings_local (in-repo copy)

This is the copy of the
`github.com/DataDog/datadog-agent/pkg/config/setup/generatedsettings` module
that in-repo builds use. `go.work`, every in-repo `replace` directive (through
the `workspace_path` attribute in `modules.yml`), and Bazel point here instead of
`../generatedsettings`.

## Do not edit the generated code

The `*.go` files other than `doc.go` (`all_settings.go`, `generated.go`,
`system_probe_settings.go`) are generated from the config schema
(`pkg/config/schema/yaml/`) and are gitignored:

- `dda inv schema.codegen` writes them, and every `dda inv` build, test, lint,
  and tidy runs it.
- Bazel generates its own copy through the `codegen_settings` target in
  `BUILD.bazel`.

Do not edit them by hand. Any change is overwritten by the next build. To
change a setting, edit the schema (see `doc/how-to/agent-schema/settings.md`).

## Committed files

- `go.mod` and `go.sum` are maintained by `dda inv tidy`. Don't edit them by
  hand.
- `doc.go` is hand-written. Its imports keep the package and its dependencies
  resolvable on a fresh checkout, before codegen has run. Update it only when the
  generated code starts depending on a new module.

The committed snapshot in `../generatedsettings` is refreshed automatically by
release automation. Do not refresh it in your PRs (see its README).
