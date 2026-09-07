# Typed e2ectl configuration

This package supplies validation, defaults and example generation for data-only Go
configuration types. It uses the existing YAML library plus the standard library;
it must not depend on Pulumi, runtime clients, credentials or the environment store.

## Declare the input contract once

```go
type Params struct {
    OS    string `yaml:"os" config:"required" enum:"linux,windows" example:"linux" description:"Target OS."`
    Arch  string `yaml:"arch" enum:"amd64,arm64" default:"amd64"`
    Nodes int    `yaml:"nodes" minimum:"0" default:"0"`
}

var Schema = configschema.Must[Params]()
```

`Compile` returns metadata errors; `Must` is for static declarations and treats invalid
metadata as a programming error. Each shared schema is compiled once. User input errors
come back from `Decode`/`DecodeNode`; they never require panic recovery.

Supported annotations:

| Tag | Meaning |
|---|---|
| `yaml` | Serialized name; `omitempty` is accepted, `-` excludes the field |
| `config:"required"` | Require presence when no default is declared; zero and false are valid values |
| `config:"secret"` | Never emit defaults, examples or example overrides for this field |
| `default` | Apply only when the field is absent |
| `example` | Value for generated starter configs, **not** a runtime default |
| `enum` | Comma-separated choices for strings |
| `pattern` | Go regexp for strings |
| `minimum`, `maximum` | Inclusive bounds for signed integers |
| `description` | Field documentation emitted as YAML comments |

Unknown tags/config flags and malformed or incompatible annotations fail schema
compilation. Defaults/examples must satisfy the automatic field rules. The initial
supported type set is strings, booleans, integers, pointers, nested structs, slices
and string-keyed maps. Null, aliases, recursive types, inline structs and custom
YAML/text codecs are deliberately unsupported rather than being silently coerced.
Additional scalar codecs can be designed
when there is a real use case; runtime clients and Pulumi options are never config fields.

## Optional `Validate(params)`

Use annotations for ordinary constraints. For relations between fields, implement a
pure typed validator:

```go
type Bounds struct {
    Min int `yaml:"min" minimum:"0" default:"1"`
    Max int `yaml:"max" minimum:"0" default:"5"`
}

type BoundsRules struct{}

func (BoundsRules) Validate(p Bounds) error {
    if p.Min > p.Max {
        return fmt.Errorf("min must not exceed max")
    }
    return nil
}

var Schema = configschema.Must[Bounds](BoundsRules{})
```

The hook sees defaults and runs only after automatic validation and typed decoding
succeed. Examples are also checked by the hook. Validators must not perform network
requests or resolve credentials, and must not put secret values into errors.

For a Pulumi scenario, put both the type and its shared validator in
`cmd/internal/envconfig/<scenario>`. The CLI and executor then use the exact same schema
and rule implementation. A second hand-written executor parameter struct is unnecessary.

Drivers may also optionally implement `Validate(params P) error`. `driver.Define` detects
that method and invokes it after the shared schema validation. Put rules that the executor
must also enforce on the **shared schema**, not only on the CLI driver. Avoid registering
the same rule in both places. Runtime availability/credential checks belong in execution,
not either validation hook.

## Normalization and transport

`Decode` returns `(typedValue, normalizedYAML, error)`. `DecodeNode` preserves the original
user-file line/column positions in automatic field errors. Neither mutates input nodes.

The normalized YAML includes explicit false/zero values and materialized defaults, even
when a struct has `omitempty`. Forward those normalized bytes rather than re-marshalling
the struct and accidentally dropping `false` before the executor re-applies a `true`
default. Common fixture settings have their own shared schema and are transported
separately with an explicit protocol version. The executor uses `DecodeResolved`,
which requires defaulted fields to already be present, then runs the same automatic
and semantic validation. Fixture JSON is checked before Go decoding can erase omission
or null presence; `{}` and `{"fakeintake": null}` are invalid normalized fixtures,
while `{"fakeintake": false}` is valid.

The secret-generation restriction is recursive: parent defaults/examples and overrides
cannot emit nested secret fields, including through maps and slices. It does not forbid
explicit runtime secrets; caller-owned runtime files must still remain private.

Examples never affect runtime defaults. An absent optional field with no default stays
absent; a required field with only an example still fails if omitted from actual input.
Unknown/duplicate keys, wrong types and multiple documents are errors. Mapping order is
not significant.

## Generated starter configs

`Example` uses declared defaults first, then examples, with explicit selector overrides
when provided (for example `agent.install`). Fields with neither are omitted unless
required, in which case generation returns an error instead of inventing a value.
Examples are emitted as active, editable values and labelled as examples; defaults are
labelled separately. Optional secret fields are omitted.

The CLI composes the environment example with the common fixture and Agent schemas.
Its explicit driver registration selects a default installer. Validate complete generated
files through the normal parser and installer rules before writing them.

Field descriptions must currently be tags: runtime reflection cannot read Go doc comments.
No generated files or handwritten per-driver YAML templates are required.
