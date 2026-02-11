# Temporary schema work

This folder is a work in progress.

First compile and run the Agent locally to get `core_schema.yaml` and `system-probe_schema.yaml` files. Those a base
schema base on the runtime of the Agent. They do not include OS differences nor reflect specific version of the Agent
(IOT, DDA, serverless, ...).

The to generate enrich the schema with the documentation and information from the `config_template.yaml`:

The datadog.yaml
```
$> ./parse_template_comment.py ../config_template.yaml core_schema.yaml core_schema_enriched.yaml 
```

The system-probe.yaml
```
$> ./parse_template_comment.py ../config_template.yaml system-probe_schema.yaml system-probe_schema_enriched.yaml
```

Then run the fix script on both:
```
$> ./fix_schema.py core_schema_enriched.yaml system-probe_schema_enriched.yaml
```


To generate a template from a enriched schema for a `build_type` and OS (same as for `pkg/config/render_config.go`)

```
$> ./generate_template.py core_schema_enriched.yaml datadog.yaml agent-py3 linux
```

To generate all the possible example from your current branch:
```
$> go run ./pkg/config/render_config.go /path/to/output/folder ./pkg/config
```

# TODO:

Feature not yet supported:
- Finding a better way to order secion when generating a template.
    + We should reorder the schema itself and use that for the generation
- Missing support for proxy env vars: This is an edge case. We could embed the env var definition directly in the doc
  string for those settings.
- Edge case for the `api_key` which is the only mandatory setting
- Some default are different between the code and the template: we need to find out which one is correct
- Type duration is not currently supported: the schema has no signal that a string will be used as a duration. Only the
  `config_template.yaml` show the information.

# generated_main folder

This folder contains all the possible output from `../render_config.go` on main for reference.
