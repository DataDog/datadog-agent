# Temporary schema work

This folder is a work in progress.

First compile and run the Agent locally to get `core_schema.yaml` and `system-probe_schema.yaml` files. Those a base
schema base on the runtime of the Agent. They do not include OS differences nor reflect specific version of the Agent
(IOT, DDA, serverless, ...).

The to generate enrich the schema with the documentation and information from the `config_template.yaml`:

```
$> python parse_template_comment.py ../config_template.yaml core_schema.yaml core_schema_enriched.yaml
```

To generate a template from a enriched schema:

```
$> python generate_template.py core_schema_enriched.yaml core.yaml
```

Rough idea of the remaining differences:
```
$> diff core.yaml ../config_template.yaml | grep -- '---' | wc -l
```

# TODO:

Feature not yet supported:
- Supporting section when generating
- Finding a better way to order secion when generating a template.
    + We should reorder the schema itself and use that for the generation
- Support `os_default`
    + some setting have differents default. We should generate the schema on all OS and merge them
    + or some hardcoded post-processing when enriching a schema is the number of `os_default` case is low.
- Missing support for proxy env vars: This is an edge case. We could embed the env var definition directly in the doc
  string for those settings.
- Edge case for the `api_key` which is the only mandatory setting
- Some port are strings in the config: this will be fixed by https://github.com/DataDog/datadog-agent/pull/44500
- Some default are different between the code and the template: we need to find out which one is correct
- Type duration is not currently supported: the schema has no signal that a string will be used as a duration. Only the
  `config_template.yaml` show the information.
- We should split `config_template.yaml` into `datadog-agent_template.yaml` and `system-probe_template.yaml`. For this we
  need to find where each use of `../render_config.go` is. Some case in `render_config.go` don't seemed to be used in the
  repo (maybe in other repo ?).

# generated_main folder

This folder contains all the possible output from `../render_config.go` on main for reference.
