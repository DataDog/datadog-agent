// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/DataDog/fastjq"
)

// jqArgumentName matches the argument names that can be bound to a jq variable ($name).
var jqArgumentName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// jqReservedPrefix is rejected on top of jqArgumentName: jq keeps its own built-in
// variables in that namespace, and a built-in wins over the binding. $__loc__ for
// instance would silently hand the transform a source location instead of the argument.
const jqReservedPrefix = "__"

// Keys of the wrapper document that carries the config and the arguments to the query.
// They are only visible to the generated prelude, never to the transform itself.
const (
	jqConfigKey    = "config"
	jqArgumentsKey = "arguments"
)

// jqTransform is a compiled jq transform together with the arguments bound to it.
type jqTransform struct {
	program   *fastjq.Program
	arguments map[string]any
}

// newJQTransform compiles transform, exposing arguments to it as named jq variables
// ($name) so that the transform stays a static program with its values supplied
// separately.
//
// fastjq has no API for injecting variables from the host, so the bindings are generated
// as a prelude in the query itself. The prelude destructures the arguments out of the
// *input document* rather than out of a literal spliced into the query text. That keeps
// argument values — which may hold secrets, see ReplaceSecrets — out of the compiled
// query, and therefore out of any parse error it might produce. Only the argument names
// reach the query, and they are validated to be plain jq identifiers first.
func newJQTransform(transform string, rawArguments json.RawMessage) (*jqTransform, error) {
	arguments := make(map[string]any)
	if len(rawArguments) > 0 {
		// Decoded through json.Number so that an argument re-encoded into the input
		// document keeps its exact literal form, rather than being rounded through
		// float64 on the way in.
		decoder := json.NewDecoder(bytes.NewReader(rawArguments))
		decoder.UseNumber()
		if err := decoder.Decode(&arguments); err != nil {
			return nil, fmt.Errorf("failed to parse jq arguments: %w", err)
		}
	}

	query := transform
	if len(arguments) > 0 {
		names := make([]string, 0, len(arguments))
		for name := range arguments {
			if !jqArgumentName.MatchString(name) || strings.HasPrefix(name, jqReservedPrefix) {
				return nil, fmt.Errorf("invalid jq argument name %q: must be a jq identifier not starting with %q", name, jqReservedPrefix)
			}
			names = append(names, name)
		}
		// Sorted so that a given set of arguments always produces the same query.
		sort.Strings(names)
		query = fmt.Sprintf(".%s as {$%s} | .%s | %s",
			jqArgumentsKey, strings.Join(names, ", $"), jqConfigKey, transform)
	}

	program, err := fastjq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("failed to compile jq transform: %w", err)
	}
	return &jqTransform{program: program, arguments: arguments}, nil
}

// run executes the transform over config and returns one value per jq output. A
// transform may legitimately yield any number of outputs, including none.
func (t *jqTransform) run(config any) ([]any, error) {
	input := config
	if len(t.arguments) > 0 {
		input = map[string]any{jqConfigKey: config, jqArgumentsKey: t.arguments}
	}

	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to encode config for the jq transform: %w", err)
	}

	results, err := t.program.RunAll(inputBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to run jq transform: %w", err)
	}

	outputs := make([]any, 0, len(results))
	for _, result := range results {
		output, err := decodeJQOutput(result)
		if err != nil {
			return nil, fmt.Errorf("failed to decode jq transform output: %w", err)
		}
		outputs = append(outputs, output)
	}
	return outputs, nil
}

// decodeJQOutput decodes a jq output document into YAML-encodable Go values. Numbers are
// decoded through json.Number so that integers stay integral: a plain json.Unmarshal
// widens every number to float64, which both renders small integers as floats and loses
// precision past 2^53.
func decodeJQOutput(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return convertJSONNumbers(value), nil
}

// convertJSONNumbers replaces every json.Number in v with the narrowest Go numeric type
// that represents it exactly, since yaml.Marshal renders json.Number as a quoted string.
func convertJSONNumbers(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for key, value := range x {
			x[key] = convertJSONNumbers(value)
		}
		return x
	case []any:
		for i, value := range x {
			x[i] = convertJSONNumbers(value)
		}
		return x
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		// Neither representable as int64 nor float64: keep the literal digits.
		return x.String()
	}
	return v
}
