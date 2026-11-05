# Validate and generate schema output

Use these workflows after changing an Agent configuration setting. The [schema overview](../../architecture/agent-schema/index.md#how-the-commands-fit-together) explains how the generated artifacts relate to the source, and the [CLI reference](../../reference/agent-schema/cli.md) lists command arguments.

## Validate changes and regenerate Go code

After editing the schema, validate it and regenerate the Go files that register settings:

```bash
dda inv schema.lint
dda inv schema.codegen
```

`schema.codegen` writes the gitignored files in `pkg/config/setup/generatedsettings_local`. Don't run `dda inv schema.codegen-snapshot` or commit changes to `pkg/config/setup/generatedsettings`. Release automation refreshes that [committed snapshot](../../architecture/agent-schema/index.md#the-generated-settings-snapshot) after each release.

## Generate configuration examples

The Agent build generates configuration examples automatically. To validate all generated examples locally, run:

```bash
dda inv schema.template-all
```

This command rewrites the examples in `pkg/config/example/`. Don't commit those changes. After each Agent release, an automated PR refreshes the examples from that release's pipeline (see `pkg/config/example/README.md`).

To preview the example for one platform without running the whole build, render it to a temporary file:

```bash
dda inv schema.template \
  --schema=./pkg/config/schema/yaml/core_schema.yaml \
  --build-type=datadog-agent \
  --os-target=linux \
  --output=/tmp/datadog.yaml.example
```

Choose an output path suitable for your operating system. See the [template command reference](../../reference/agent-schema/cli.md#schematemplate) for supported build types and platforms.

## Locate a setting

Find a setting's schema node and the source file and line where it is declared:

```bash
dda inv -- schema.locate apm_config.enabled
```

To list every setting whose path ends in `enabled`, use a pattern:

```bash
dda inv -- schema.locate '*enabled'
```

See the [lookup command reference](../../reference/agent-schema/cli.md#schemalocate) for regular expressions, schema selection, and JSON output.
