// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJQTransformArgumentNames(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		wantErr   string
	}{
		{name: "plain identifier", arguments: `{"api_key": "x"}`},
		{name: "leading underscore", arguments: `{"_private": "x"}`},
		{name: "digits after letter", arguments: `{"tag2": "x"}`},
		{name: "leading digit", arguments: `{"2tag": "x"}`, wantErr: `invalid jq argument name "2tag"`},
		{name: "dash", arguments: `{"env-tag": "x"}`, wantErr: `invalid jq argument name "env-tag"`},
		{name: "dot", arguments: `{"a.b": "x"}`, wantErr: `invalid jq argument name "a.b"`},
		{name: "empty", arguments: `{"": "x"}`, wantErr: `invalid jq argument name ""`},
		{name: "space", arguments: `{"a b": "x"}`, wantErr: `invalid jq argument name "a b"`},
		{name: "jq syntax", arguments: `{"a | error(\"x\")": "y"}`, wantErr: `invalid jq argument name`},
		// jq resolves its own built-in variables ahead of the binding, so a name in that
		// namespace would silently shadow the supplied value.
		{name: "jq builtin variable", arguments: `{"__loc__": "x"}`, wantErr: `invalid jq argument name "__loc__"`},
		{name: "reserved prefix", arguments: `{"__internal": "x"}`, wantErr: `invalid jq argument name "__internal"`},
		// jq keywords are fine: variable names live in their own namespace.
		{name: "keyword if", arguments: `{"if": "x"}`},
		{name: "keyword end", arguments: `{"end": "x"}`},
		{name: "keyword def", arguments: `{"def": "x"}`},
		{name: "keyword reduce", arguments: `{"reduce": "x"}`},
		{name: "keyword not", arguments: `{"not": "x"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The transform references no variable, so only name validation can fail.
			transform, err := newJQTransform(`.`, json.RawMessage(tt.arguments))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, transform)
		})
	}
}

func TestNewJQTransformInvalidArgumentsJSON(t *testing.T) {
	_, err := newJQTransform(`.`, json.RawMessage(`{not json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse jq arguments")
}

func TestNewJQTransformInvalidTransform(t *testing.T) {
	_, err := newJQTransform(`.foo |`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to compile jq transform")
}

// TestNewJQTransformDoesNotLeakArgumentValues is the reason arguments are bound from the
// input document rather than spliced into the query text: argument values carry
// substituted secrets, and a compile error must not echo them.
func TestNewJQTransformDoesNotLeakArgumentValues(t *testing.T) {
	const secret = "super-secret-api-key"

	// A transform that cannot compile, so the error path is exercised.
	_, err := newJQTransform(`.foo = $api_key | (`, json.RawMessage(`{"api_key": "`+secret+`"}`))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)

	// Same for a transform referencing an argument that was not supplied.
	_, err = newJQTransform(`.foo = $missing`, json.RawMessage(`{"api_key": "`+secret+`"}`))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
}

// TestJQTransformArgumentValuesAreInert checks that an argument value shaped like jq
// source is treated as data. Values never reach the parser, so they cannot inject code.
func TestJQTransformArgumentValuesAreInert(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "string break out", value: `") | error("pwned`},
		{name: "interpolation", value: `\(1+1)`},
		{name: "pipe and filter", value: `. | keys`},
		{name: "quotes and backslashes", value: `a "b" \ c`},
		{name: "newline", value: "line1\nline2"},
		{name: "unicode", value: "日本語"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arguments, err := json.Marshal(map[string]any{"injected": tt.value})
			require.NoError(t, err)

			transform, err := newJQTransform(`.value = $injected`, arguments)
			require.NoError(t, err)

			outputs, err := transform.run(map[string]any{})
			require.NoError(t, err)
			require.Len(t, outputs, 1)
			assert.Equal(t, map[string]any{"value": tt.value}, outputs[0])
		})
	}
}

// TestJQTransformArgumentNamesShadowingWrapperKeys checks that arguments named after the
// wrapper document's own keys still resolve to the argument value, and that the transform
// still sees the config rather than the wrapper.
func TestJQTransformArgumentNamesShadowingWrapperKeys(t *testing.T) {
	arguments := json.RawMessage(`{"config": "arg-config", "arguments": "arg-arguments"}`)
	transform, err := newJQTransform(`.seen_config = $config | .seen_arguments = $arguments`, arguments)
	require.NoError(t, err)

	outputs, err := transform.run(map[string]any{"original": true})
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	assert.Equal(t, map[string]any{
		"original":       true,
		"seen_config":    "arg-config",
		"seen_arguments": "arg-arguments",
	}, outputs[0])
}

// TestJQTransformKeywordArgumentNames checks that an argument named after a jq keyword
// still binds to its value, since the generated prelude puts the name after a `$`.
func TestJQTransformKeywordArgumentNames(t *testing.T) {
	for _, keyword := range []string{"if", "then", "else", "end", "def", "as", "reduce", "foreach", "try", "catch", "label", "and", "or", "not", "import", "include", "empty", "null", "true", "false"} {
		t.Run(keyword, func(t *testing.T) {
			arguments, err := json.Marshal(map[string]any{keyword: "bound"})
			require.NoError(t, err)

			transform, err := newJQTransform(`.out = $`+keyword, arguments)
			require.NoError(t, err)

			outputs, err := transform.run(map[string]any{})
			require.NoError(t, err)
			require.Len(t, outputs, 1)
			assert.Equal(t, map[string]any{"out": "bound"}, outputs[0])
		})
	}
}

func TestJQTransformOutputCounts(t *testing.T) {
	t.Run("single output", func(t *testing.T) {
		transform, err := newJQTransform(`.foo = "baz"`, nil)
		require.NoError(t, err)
		outputs, err := transform.run(map[string]any{"foo": "bar"})
		require.NoError(t, err)
		assert.Equal(t, []any{map[string]any{"foo": "baz"}}, outputs)
	})

	t.Run("multiple outputs", func(t *testing.T) {
		transform, err := newJQTransform(`.items[] | {value: .}`, nil)
		require.NoError(t, err)
		outputs, err := transform.run(map[string]any{"items": []any{"a", "b"}})
		require.NoError(t, err)
		assert.Equal(t, []any{
			map[string]any{"value": "a"},
			map[string]any{"value": "b"},
		}, outputs)
	})

	t.Run("no output", func(t *testing.T) {
		transform, err := newJQTransform(`empty`, nil)
		require.NoError(t, err)
		outputs, err := transform.run(map[string]any{"foo": "bar"})
		require.NoError(t, err)
		assert.Empty(t, outputs)
	})

	t.Run("no output with arguments", func(t *testing.T) {
		transform, err := newJQTransform(`.items[] | select(. == $wanted)`, json.RawMessage(`{"wanted": "z"}`))
		require.NoError(t, err)
		outputs, err := transform.run(map[string]any{"items": []any{"a", "b"}})
		require.NoError(t, err)
		assert.Empty(t, outputs)
	})
}

// TestJQTransformRuntimeErrorRedactsArguments covers the other half of the secret story:
// argument values reach the engine as input data, and jq names the offending value in most
// runtime errors, so those errors must not carry the value out to the logs.
func TestJQTransformRuntimeErrorRedactsArguments(t *testing.T) {
	const secret = "super-secret-api-key"

	tests := []struct {
		name      string
		transform string
	}{
		{name: "type error in arithmetic", transform: `.foo = ($api_key + 1)`},
		{name: "tonumber", transform: `$api_key | tonumber`},
		{name: "index array with string", transform: `.list = [1,2,3] | .list[$api_key]`},
		{name: "explicit error", transform: `error($api_key)`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arguments, err := json.Marshal(map[string]string{"api_key": secret})
			require.NoError(t, err)

			transform, err := newJQTransform(tt.transform, arguments)
			require.NoError(t, err)

			_, err = transform.run(map[string]any{})
			require.Error(t, err)
			assert.NotContains(t, err.Error(), secret)
			assert.Contains(t, err.Error(), "<redacted>")
		})
	}
}

// TestJQTransformRedactsNestedArguments checks that a secret nested inside an object or
// array argument is redacted too, not just a top-level string.
func TestJQTransformRedactsNestedArguments(t *testing.T) {
	const secret = "nested-secret-value"

	arguments, err := json.Marshal(map[string]any{
		"apm": map[string]any{"credentials": []any{"public", secret}},
	})
	require.NoError(t, err)

	transform, err := newJQTransform(`.out = ($apm.credentials[1] + 1)`, arguments)
	require.NoError(t, err)

	_, err = transform.run(map[string]any{})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
}

// TestJQTransformShortArgumentsAreNotRedacted checks that redaction leaves ordinary short
// values alone, so errors stay readable.
func TestJQTransformShortArgumentsAreNotRedacted(t *testing.T) {
	transform, err := newJQTransform(`.out = ($mode + 1)`, json.RawMessage(`{"mode": "fast"}`))
	require.NoError(t, err)

	_, err = transform.run(map[string]any{})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "<redacted>")
}

// TestJQTransformOutputCap bounds a runaway generator. fastjq cannot be cancelled, so the
// cap is the only protection against a remotely authored transform that never stops.
func TestJQTransformOutputCap(t *testing.T) {
	transform, err := newJQTransform(`range(100000) | {value: .}`, nil)
	require.NoError(t, err)

	_, err = transform.run(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than 1024 output documents")
}

// TestJQTransformAtOutputCap checks the cap admits a transform that sits exactly on it.
func TestJQTransformAtOutputCap(t *testing.T) {
	transform, err := newJQTransform(`range(1024) | {value: .}`, nil)
	require.NoError(t, err)

	outputs, err := transform.run(map[string]any{})
	require.NoError(t, err)
	assert.Len(t, outputs, maxJQOutputs)
}

func TestJQTransformRuntimeError(t *testing.T) {
	transform, err := newJQTransform(`.foo.bar`, nil)
	require.NoError(t, err)

	_, err = transform.run(map[string]any{"foo": "bar"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run jq transform")
}

func TestJQTransformUnencodableConfig(t *testing.T) {
	transform, err := newJQTransform(`.`, nil)
	require.NoError(t, err)

	_, err = transform.run(map[string]any{"ch": make(chan int)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to encode config for the jq transform")
}

// TestDecodeJQOutputNumbers pins the number handling of the JSON -> Go step. gojq handed
// back Go values directly; going through JSON means integers must not silently widen to
// floats, which would change how they are rendered in YAML.
func TestDecodeJQOutputNumbers(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected any
	}{
		{name: "small integer", json: `4`, expected: int64(4)},
		{name: "negative integer", json: `-7`, expected: int64(-7)},
		{name: "zero", json: `0`, expected: int64(0)},
		{name: "max int64", json: `9223372036854775807`, expected: int64(math.MaxInt64)},
		{name: "min int64", json: `-9223372036854775808`, expected: int64(math.MinInt64)},
		{name: "beyond float64 exact range", json: `9007199254740993`, expected: int64(9007199254740993)},
		{name: "float", json: `1.5`, expected: 1.5},
		{name: "float with integral value", json: `1.0`, expected: 1.0},
		{name: "exponent", json: `1e3`, expected: 1000.0},
		{name: "beyond int64", json: `99999999999999999999`, expected: 1e20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := decodeJQOutput([]byte(tt.json))
			require.NoError(t, err)
			assert.Equal(t, tt.expected, value)
		})
	}
}

func TestDecodeJQOutputNested(t *testing.T) {
	value, err := decodeJQOutput([]byte(`{"a":[1,2.5,{"b":3}],"c":"str","d":null,"e":true}`))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"a": []any{int64(1), 2.5, map[string]any{"b": int64(3)}},
		"c": "str",
		"d": nil,
		"e": true,
	}, value)
}

// TestJQTransformLargeIntegerRoundTrip checks that an integer too large for float64 to
// hold exactly survives argument binding, the query, and the JSON decode.
func TestJQTransformLargeIntegerRoundTrip(t *testing.T) {
	const large = int64(9007199254740993) // 2^53 + 1

	transform, err := newJQTransform(`.value = $big`, json.RawMessage(`{"big": 9007199254740993}`))
	require.NoError(t, err)

	outputs, err := transform.run(map[string]any{})
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	assert.Equal(t, map[string]any{"value": large}, outputs[0])
}

// TestNewJQTransformArgumentOrderIsStable checks that the generated prelude does not
// depend on Go's map iteration order.
func TestNewJQTransformArgumentOrderIsStable(t *testing.T) {
	arguments := json.RawMessage(`{"zeta": 1, "alpha": 2, "mid": 3}`)
	transform := `.a = $alpha | .m = $mid | .z = $zeta`

	var first []any
	for i := 0; i < 20; i++ {
		compiled, err := newJQTransform(transform, arguments)
		require.NoError(t, err)
		outputs, err := compiled.run(map[string]any{})
		require.NoError(t, err)
		if i == 0 {
			first = outputs
			continue
		}
		assert.Equal(t, first, outputs)
	}
}

// TestJQTransformNoArgumentsSkipsWrapper documents that a transform without arguments
// runs directly against the config, with no wrapper document in the way.
func TestJQTransformNoArgumentsSkipsWrapper(t *testing.T) {
	transform, err := newJQTransform(`keys`, nil)
	require.NoError(t, err)

	outputs, err := transform.run(map[string]any{"only": 1})
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	assert.Equal(t, []any{"only"}, outputs[0])

	// With arguments the wrapper exists, but the transform must still only see the config.
	withArgs, err := newJQTransform(`keys`, json.RawMessage(`{"unused": 1}`))
	require.NoError(t, err)

	outputs, err = withArgs.run(map[string]any{"only": 1})
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	assert.Equal(t, []any{"only"}, outputs[0])
}

// TestJQTransformComments documents a compatibility gap with gojq: fastjq does not
// support jq comments. Transforms are authored remotely, so this is worth pinning.
func TestJQTransformComments(t *testing.T) {
	for _, transform := range []string{
		`.foo = "baz" # set foo`,
		"# set foo\n.foo = \"baz\"",
	} {
		_, err := newJQTransform(transform, nil)
		require.Error(t, err, "expected %q to be rejected", transform)
		assert.Contains(t, strings.ToLower(err.Error()), "compile")
	}
}
