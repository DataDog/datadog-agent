// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"encoding/json"
	"errors"
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

// maxJQOutputs bounds how many YAML documents a single transform may produce. Real
// transforms emit one; the cap only exists to stop a runaway generator.
const maxJQOutputs = 1024

// minRedactedArgumentLength is the shortest argument value worth redacting out of an error
// message. Redacting very short values would garble errors without protecting anything —
// a secret is never one or two characters.
const minRedactedArgumentLength = 6

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
// query, and therefore out of the compile errors it can produce. Only the argument names
// reach the query, and they are validated to be plain jq identifiers first.
//
// Runtime errors are a separate matter: jq reports the offending value, and arguments are
// ordinary input values, so run redacts them out of the error it returns.
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

	// Streamed rather than collected with RunAll: a transform is remotely authored, and a
	// generator such as `range(100000000)` would otherwise materialise every output
	// before this function could reject it. There is no way to cancel a running fastjq
	// program, so the cap is the only bound available.
	//
	// The cap does not cover unbounded *recursion*: a transform such as `def f: f; f`
	// overflows the goroutine stack, which is a fatal runtime error that recover cannot
	// catch, so it takes the process down. That needs a depth limit inside the engine;
	// there is nothing to be done about it from here.
	var outputs []any
	err = t.program.RunFunc(inputBytes, func(result []byte) error {
		if len(outputs) >= maxJQOutputs {
			return fmt.Errorf("transform produced more than %d output documents", maxJQOutputs)
		}
		output, err := decodeJQOutput(result)
		if err != nil {
			return fmt.Errorf("cannot decode output: %w", err)
		}
		outputs = append(outputs, output)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to run jq transform: %w", t.redactArguments(err))
	}
	return outputs, nil
}

// redactArguments rewrites err so that no argument value appears in its text. jq names the
// offending value in most runtime errors ("cannot add ... and ..."), and arguments hold
// secrets substituted by ReplaceSecrets, so the raw error must not be logged as-is.
func (t *jqTransform) redactArguments(err error) error {
	if len(t.arguments) == 0 {
		return err
	}
	message := err.Error()
	for _, value := range collectArgumentStrings(t.arguments, nil) {
		message = strings.ReplaceAll(message, value, "<redacted>")
	}
	if message == err.Error() {
		return err
	}
	return errors.New(message)
}

// collectArgumentStrings gathers every string leaf in v that is long enough to be worth
// redacting, including those nested inside object and array arguments.
func collectArgumentStrings(v any, into []string) []string {
	switch x := v.(type) {
	case map[string]any:
		// Sorted so that redaction is deterministic when values overlap.
		keys := make([]string, 0, len(x))
		for key := range x {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			into = collectArgumentStrings(x[key], into)
		}
	case []any:
		for _, value := range x {
			into = collectArgumentStrings(value, into)
		}
	case string:
		if len(x) >= minRedactedArgumentLength {
			into = append(into, x)
		}
	}
	return into
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
