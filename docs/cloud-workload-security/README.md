How to work with and generate CWS documentation
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

## Agent Expressions - SECL

The agent expressions documentation is based on the following files:

- `pkg/security/model/model.go` - the source code of the SECL model containing the event types and fields documentation
- `docs/cloud-workload-security/secl.json` - the json representing the SECL model extracted from the source code
- `docs/cloud-workload-security/scripts/templates/agent_expressions.md` - the template used for the final generation

### What file should I edit ?

- First table (`Triggers`): comments on the `Event` struct in the `model.go` file

For example:
```go
Capset CapsetEvent `field:"capset" event:"capset"` // [7.27] [Process] A process changed its capacity set
```
Will generate:

| SECL Event | Type | Definition | Agent Version |
| ---------- | ---- | ---------- | ------------- |
| `capset` | Process | A process changed its capacity set | 7.27 |

----

- One of the `Event types` table: comments on the corresponding structure in the `model.go` file

For example:
```go
type FileFields struct {
	...
	CTime uint64 `field:"change_time"` // Change time of the file
	...
}
```
Will generate this field for all event containing a File sub-event, for example:

| Property | Type | Definition |
| -------- | ---- | ---------- |
| `chmod.file.change_time` | int | Change time of the file |

- The rest of the file is copied verbatim from the template file (modulo the `raw` tags, see [Jinja 2 templates](#jinja2-templates))

## Backend event

The CWS part of the agent sends event to the backend. Those events conform to a JSON schema (this is tested in functional tests of the agent). The documentation is based on the following files:

- `pkg/security/probe/serializers.go` - the serializers used to output events
- `docs/cloud-workload-security/backend.schema.json` - the json schema of the event
- `docs/cloud-workload-security/scripts/templates/backend.md` - the template used for the final generation

### Which file should I edit ?

- To change the documentation of one of the field in the schema, please edit the correct field in `pkg/security/probe/serializers.go`. The documentation of a field is added through the `jsonschema_description` tag of the field.

For example:
```go
Path string `json:"path,omitempty" jsonschema_description:"File path"`
```
The field `Path` (`path` in the json file) has a description/documentation of content "File Path".

---

- The rest of the file is copied verbatim from the template file (modulo the `raw` tags, see [Jinja 2 templates](#jinja2-templates))

## Jinja2 templates

The template are written in [Jinja2](https://jinja.palletsprojects.com/en/3.0.x/), a simple and well-known templating engine.

One point to keep in mind: the template is used to generate a file that is in itself a template for the hugo documentation site. This requires escaping `{`; for example, to start a code-block:

```
{% raw %}
{{< code-block lang="javascript" >}}
{% endraw %}
```

## How to generate the documentation ?

### Using Docker

From the root of the datadog-agent repository please run:
```sh
./docs/cloud-workload-security/scripts/generate_documentation_docker.sh
```
To run all documentation generation steps in a docker container (thus skipping the requirement of setting up a development environment).

### Manual steps

#### Requirements

- Golang (see `go.mod` for the minimal version supported)
- Python
	- `pip install -r requirements.txt` to install dependencies


#### Steps

If a `*.go` file in `pkg/security` has been edited you will first need to generate the `*.json` files.
Please run:
```sh
go generate ./pkg/security/...
# or only the specific file
go generate ./path/to/the/touched/file
```

To generate the final markdown files (at the root of `docs/cloud-workload-security`) please run:
```sh
inv -e security-agent.generate-documentation
```
