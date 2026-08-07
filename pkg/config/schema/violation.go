// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schema

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// Violation describes a schema failure for someone editing datadog.yaml, rather than for
// someone reading JSON Schema. The library's own message ("got string, want boolean") names
// our internal model instead of the customer's problem, which is why it is not used directly.
type Violation struct {
	// Pointer is the RFC 6901 location, e.g. /apm_config/enabled. Kept so a consumer can
	// attach the violation to the line it came from.
	Pointer string
	// Key is the dotted YAML key the customer actually typed, e.g. apm_config.enabled.
	Key string
	// Message states what the setting needs and what was found instead.
	Message string
	// Fix is one imperative sentence naming its own setting, so it reads correctly both as a
	// remediation step and on its own beside a config line.
	Fix string
}

// goDuration matches the Go duration literals used by every duration-valued setting.
// Only 60 of the ~92 duration settings carry `format: duration`, and format assertion is not
// enabled for 2020-12, so the shape of the setting's default is the reliable signal.
var goDuration = regexp.MustCompile(`^-?\d+(\.\d+)?(ns|us|µs|ms|s|m|h)([0-9.]+(ns|us|µs|ms|s|m|h))*$`)

// numeric matches a bare number written as text, i.e. the Helm/envsubst quoting mistake.
var numeric = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// apm_config.max_memory is `type: number` with `default: "5e+08"` — the one setting whose
// default fails its own schema. Offering it as an example would hand back an invalid value.
const defaultDisagreesWithType = "apm_config.max_memory"

var truthy = map[string]bool{"yes": true, "y": true, "on": true, "1": true}
var falsy = map[string]bool{"no": true, "n": true, "off": true, "0": true}

// ValidateCoreConfigDetailed validates against the core agent schema and describes each failure
// in terms the customer can act on. ValidateCoreConfig remains the plain-string form.
func ValidateCoreConfigDetailed(config interface{}) ([]Violation, error) {
	sch, err := coreSchemaGetter()
	if err != nil {
		return nil, err
	}
	leaves, err := validationLeaves(sch, config)
	if err != nil {
		return nil, err
	}
	violations := make([]Violation, 0, len(leaves))
	for _, leaf := range leaves {
		violations = append(violations, describe(leaf, sch, config))
	}
	return violations, nil
}

// validationLeaves returns the innermost validation errors, which are the ones that name a
// concrete instance location rather than a wrapping schema.
func validationLeaves(sch *jsonschema.Schema, config interface{}) ([]*jsonschema.ValidationError, error) {
	if sch == nil {
		return nil, errNoSchema
	}
	err := sch.Validate(config)
	if err == nil {
		return nil, nil
	}
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return nil, nil
	}
	var out []*jsonschema.ValidationError
	collectLeaves(ve, &out)
	return out, nil
}

func collectLeaves(ve *jsonschema.ValidationError, out *[]*jsonschema.ValidationError) {
	if len(ve.Causes) == 0 {
		*out = append(*out, ve)
		return
	}
	for _, cause := range ve.Causes {
		collectLeaves(cause, out)
	}
}

func describe(ve *jsonschema.ValidationError, root *jsonschema.Schema, config interface{}) Violation {
	pointer := "/" + strings.Join(escapeTokens(ve.InstanceLocation), "/")
	if len(ve.InstanceLocation) == 0 {
		pointer = ""
	}
	key := dottedKey(ve.InstanceLocation)
	node := schemaNodeAt(root, ve.InstanceLocation)
	value := valueAt(config, ve.InstanceLocation)
	want := wantedType(ve, node)

	// A bare `logs_enabled:` line resolves to nil. Nothing in the schema accepts null, so this
	// is the most common failure — and "must be boolean" describes a value never written.
	if value == nil {
		return Violation{
			Pointer: pointer,
			Key:     key,
			Message: key + " was left empty, so the Agent cannot use it.",
			Fix: fmt.Sprintf("Give `%s` a value such as `%s`, or delete the line to keep the default.",
				key, example(key, node, want)),
		}
	}

	return Violation{
		Pointer: pointer,
		Key:     key,
		Message: fmt.Sprintf("%s must be %s, but it is %s.", key, wantPhrase(node, want), renderValue(value)),
		Fix:     fix(key, node, want, value, example(key, node, want)),
	}
}

// wantedType prefers the schema node's own type; kind.Type carries it too but only as the set
// of acceptable types, and every node in this schema declares exactly one.
func wantedType(ve *jsonschema.ValidationError, node *jsonschema.Schema) string {
	if t, ok := ve.ErrorKind.(*kind.Type); ok && len(t.Want) == 1 {
		return t.Want[0]
	}
	if node != nil && node.Types != nil {
		if ts := node.Types.ToStrings(); len(ts) == 1 {
			return ts[0]
		}
	}
	return ""
}

func wantPhrase(node *jsonschema.Schema, want string) string {
	if isDuration(node) {
		return fmt.Sprintf("a duration such as %q", *node.Default)
	}
	switch want {
	case "boolean":
		return "true or false"
	case "integer":
		return "a whole number"
	case "number":
		return "a number"
	case "array":
		if itemsOf(node) != nil && typeOf(itemsOf(node)) == "string" {
			return "a list of text values"
		}
		return "a list"
	case "object":
		return "a set of key/value pairs"
	default:
		return "text"
	}
}

func renderValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("the text %q", v)
	case bool:
		return strconv.FormatBool(v)
	case float64:
		return "the number " + strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return fmt.Sprintf("the number %d", v)
	case int64:
		return fmt.Sprintf("the number %d", v)
	case []interface{}:
		return "a list"
	case map[string]interface{}:
		return "a set of key/value pairs"
	}
	return fmt.Sprintf("%v", value)
}

// example returns a value the customer can paste. The schema default covers all but 11 settings,
// but roughly a fifth of those are "" / [] / {}, so fall back to a shape rather than suggest an
// empty token.
func example(key string, node *jsonschema.Schema, want string) string {
	if node != nil && node.Default != nil && key != defaultDisagreesWithType {
		if rendered, ok := renderDefault(*node.Default); ok {
			return rendered
		}
	}
	switch want {
	case "boolean":
		return "true"
	case "integer", "number":
		return "10"
	case "array":
		return `["first", "second"]`
	case "object":
		return "key: value"
	default:
		return `"value"`
	}
}

func renderDefault(d interface{}) (string, bool) {
	switch v := d.(type) {
	case string:
		if v == "" {
			return "", false
		}
	case float64:
		if v == 0 {
			return "", false
		}
	case []interface{}:
		if len(v) == 0 {
			return "", false
		}
	case map[string]interface{}:
		if len(v) == 0 {
			return "", false
		}
	case nil:
		return "", false
	}
	encoded, err := json.Marshal(d)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

// fix suggests a correction only where the customer's intent is unambiguous. A confident wrong
// suggestion is worse than a generic one: telling someone to quote a duration yields "30", which
// passes validation and is still wrong at runtime.
func fix(key string, node *jsonschema.Schema, want string, value interface{}, example string) string {
	if s, ok := value.(string); ok {
		trimmed := strings.TrimSpace(s)
		lower := strings.ToLower(trimmed)
		// YAML 1.2 resolves yes/no/on/off to text, so they look boolean but are not
		if want == "boolean" && truthy[lower] {
			return fmt.Sprintf("Set `%s: true`.", key)
		}
		if want == "boolean" && falsy[lower] {
			return fmt.Sprintf("Set `%s: false`.", key)
		}
		if want == "boolean" && (lower == "true" || lower == "false") {
			return fmt.Sprintf("Remove the quotes: `%s: %s`.", key, lower)
		}
		if (want == "integer" || want == "number") && numeric.MatchString(trimmed) {
			return fmt.Sprintf("Remove the quotes: `%s: %s`.", key, trimmed)
		}
		// the reverse duration mistake: a duration literal on a plain-number setting
		if (want == "integer" || want == "number") && goDuration.MatchString(trimmed) {
			return fmt.Sprintf("`%s` takes a plain number, not a duration — for example `%s: %s`.", key, key, example)
		}
		if want == "array" && strings.ContainsAny(trimmed, ", \t") {
			items := strings.FieldsFunc(trimmed, func(r rune) bool {
				return r == ',' || r == ' ' || r == '\t'
			})
			encoded, err := json.Marshal(items)
			if err == nil {
				return fmt.Sprintf("Write it as a list: `%s: %s`.", key, encoded)
			}
		}
	}
	// a bare number on a duration setting: name the unit rather than the quotes
	if isDuration(node) {
		if n, ok := asNumber(value); ok {
			return fmt.Sprintf("If you meant %s seconds, set `%s: \"%ss\"`.", n, key, n)
		}
	}
	// a section groups other settings, so suggesting `key: value` for it makes no sense
	if want == "object" && node != nil && len(node.Properties) > 0 {
		return fmt.Sprintf("`%s` groups other settings — indent them beneath it instead of giving it a value.", key)
	}
	return fmt.Sprintf("Set `%s` to something like `%s`.", key, example)
}

func asNumber(value interface{}) (string, bool) {
	switch v := value.(type) {
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	}
	return "", false
}

func isDuration(node *jsonschema.Schema) bool {
	if node == nil || node.Default == nil || typeOf(node) != "string" {
		return false
	}
	s, ok := (*node.Default).(string)
	return ok && goDuration.MatchString(s)
}
