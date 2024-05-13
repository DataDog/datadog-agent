How to edit and generate CWS documentation
==========================================

## Global folder structure

```
docs/cloud-workload-security/
    # scripts and templates used to generate the final documentation
--- scripts/
--- --- templates/ # jinja2 templates
--- --- *.py # generations scripts

    # json schema of the event uploaded to the backend
--- backend.schema.json

    # event types and fields of the SECL language
--- secl.json

    # final documentation files
--- agent_expressions.md # SECL part
--- backend.md # backend event part
```

### Agent Expressions - SECL

The Agent expressions documentation is based on the following files:

- `pkg/security/secl/model/model.go` - the source code of the SECL model containing the event types and fields documentation
- `docs/cloud-workload-security/secl.json` - the JSON representing the SECL model extracted from the source code
- `docs/cloud-workload-security/scripts/templates/agent_expressions.md` - the template used for the final generation

#### Editing files

For example, the first table, `Triggers`, comments on the `Event` struct in the `model.go` file:

```go
Capset CapsetEvent `field:"capset" event:"capset"` // [7.27] [Process] A process changed its capacity set
```

These lines generate in the following table:

| SECL Event | Type | Definition | Agent Version |
| ---------- | ---- | ---------- | ------------- |
| `capset` | Process | A process changed its capacity set | 7.27 |

To edit the definition for documentation, edit the comment. For example, `A process changed its capacity set`.

If you need to edit the type, edit `[Process]`. To edit the version, edit `[7.27]`.

As an additional example, the following `Event types` table comments on the corresponding structure in the `model.go` file:

```go
type FileFields struct {
	...
	CTime uint64 `field:"change_time"` // Change time of the file
	...
}
```

These lines generate this field for all events containing a File sub-event, for example:

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `chmod.file.change_time` | int | Change time of the file |

The rest of the file is copied verbatim from the template file (modulo the `raw` tags, see [Jinja 2 templates](#jinja2-templates)).

### Backend event

The Cloud Workload Security (CWS) part of the Agent sends events to the backend. Those events conform to a JSON schema (this is tested in functional tests of the Agent). This documentation is based on the following files:

- `pkg/security/probe/serializers.go` - the serializers used to output events
- `docs/cloud-workload-security/backend.schema.json` - the JSON schema of the event
- `docs/cloud-workload-security/scripts/templates/backend.md` - the template used for the final generation

### Editing files

To change the documentation of one of the fields in the schema, edit the correct field in `pkg/security/probe/serializers.go`. The documentation of a field is added through the commont of the field.

For example:

```go
Path string `json:"path,omitempty" jsonschema_description:"File path"`
```

The field `Path` (`path` in the JSON file) has a description/documentation of content "File Path".

The rest of the file is copied verbatim from the template file (modulo the `raw` tags, see [Jinja 2 templates](#jinja2-templates)).

### Jinja2 templates

The templates are written in [Jinja2](https://jinja.palletsprojects.com/en/3.0.x/), a simple and well-known templating engine.

**Note**: The template is used to generate a file that is in itself a template for the hugo documentation site. This requires escaping `{`. For example, to start a code-block:

```
{% raw %}
{{< code-block lang="javascript" >}}
{% endraw %}
```

## Generating the documentation

### Manual steps (for Linux environments only)

#### Requirements

- Golang (see `go.mod` for the minimal version supported)
- Python, install dependencies with:
	- `pip install -r requirements.txt`
	- `pip install -r docs/cloud-workload-security/scripts/requirements-docs.txt`


#### Steps

If a `*.go` file in `pkg/security` has been edited you will first need to generate the `*.json` files.
Please run:
```sh
inv -e security-agent.cws-go-generate
# or only the specific file
go generate ./path/to/the/touched/file
```

To generate the final markdown files please run:
```sh
inv -e security-agent.generate-cws-documentation
```
