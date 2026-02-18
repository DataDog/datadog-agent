# Temporary schema work

This folder is a work in progress.

First compile and run the Agent locally to get `core_schema.yaml` and `system-probe_schema.yaml` files. Those a base
schema base on the runtime of the Agent. They do not include OS differences nor reflect specific version of the Agent
(IOT, DDA, serverless, ...).

The to generate enrich the schema with the documentation and information from the `config_template.yaml`:

```
$> ./parse_template_comment.py ../config_template.yaml core_schema.yaml core_schema_enriched.yaml
$> ./parse_template_comment.py ../config_template.yaml system-probe_schema.yaml system-probe_schema_enriched.yaml
```

Then run the fix script on both:
```
$> ./fix_schema.py core_schema_enriched.yaml system-probe_schema_enriched.yaml
```

To generate a template from a enriched schema for a `build_type` and OS (same as for `pkg/config/render_config.go`)

```
$> ./generate_template.py <schema> <output file> <build_type> <target platform>
```

To generate all the possible example from your current branch with the old and new system:
```
# using the old templating system
$> go run ./pkg/config/render_config.go /path/to/output/folder ./pkg/config
# using the schema
$> ./generate_template.py core_schema_enriched.yaml system-probe_schema_enriched.yaml /path/to/output/folder
```

# TODO:

Feature not yet supported:
- Finding a better way to order secion when generating a template.
    + We should reorder the schema itself and use that for the generation
