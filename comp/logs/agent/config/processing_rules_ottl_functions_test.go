// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"context"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
)

// Upstream factories remain test-only so dependency updates cannot silently
// change the behavior of the subset copied into internal/ottlfuncs.
func TestOTTLFunctionsMatchUpstream(t *testing.T) {
	for _, test := range []struct {
		pattern, input string
	}{
		{`IsMatch(attributes["value"], "^sec")`, `{"value":"secret"}`},
		{`IsMatch(attributes["value"], attributes["regex"])`, `{"value":"secret","regex":"^sec"}`},
		{`IsMatch(attributes["value"], attributes["regex"])`, `{"value":"secret","regex":"["}`},
		{`IsMatch(attributes["value"], "[")`, `{}`},
		{`HasPrefix(body, "hello") and HasSuffix(body, "world")`, `hello world`},
		{`HasPrefix(attributes["value"], "sec")`, `{"value":42}`},
		{`IsString(attributes["value"])`, `{"value":"hello"}`},
		{`IsInt(attributes["value"])`, `{"value":9007199254740993}`},
		{`IsDouble(attributes["value"])`, `{"value":1.5}`},
		{`IsBool(attributes["value"])`, `{"value":true}`},
		{`IsMap(attributes["value"])`, `{"value":{"nested":1}}`},
		{`IsList(attributes["value"])`, `{"value":[1,"two"]}`},
		{`Len(attributes["value"]) == 2`, `{"value":[1,2]}`},
		{`Len(attributes["value"]) == 1`, `{"value":{"key":1}}`},
		{`Len(attributes["value"]) == 5`, `{"value":"hello"}`},
		{`Len(attributes["value"]) == 0`, `{"value":42}`},
		{`Int(attributes["value"]) == 503`, `{"value":"503"}`},
		{`Int(attributes["value"]) == 503`, `{"value":"unknown"}`},
		{`Double(attributes["value"]) == 1.5`, `{"value":"1.5"}`},
		{`String(attributes["value"]) == "503"`, `{"value":503}`},
		{`IsMap(attributes["value"])`, `not JSON`},
	} {
		t.Run(test.pattern+test.input, func(t *testing.T) {
			parser, err := ottl.NewParser(ottlfuncs.StandardFuncs[*OTTLContext](), parseOTTLPath, component.TelemetrySettings{Logger: zap.NewNop()})
			require.NoError(t, err)
			reference, referenceErr := parser.ParseCondition(test.pattern)
			actual, actualErr := compileOTTLCondition(test.pattern)
			if referenceErr != nil {
				require.EqualError(t, actualErr, referenceErr.Error())
				return
			}
			require.NoError(t, actualErr)
			expectedContext := AcquireOTTLContext([]byte(test.input))
			defer ReleaseOTTLContext(expectedContext)
			actualContext := AcquireOTTLContext([]byte(test.input))
			defer ReleaseOTTLContext(actualContext)
			expected, referenceErr := reference.Eval(context.Background(), expectedContext)
			result, actualErr := actual.Eval(context.Background(), actualContext)
			if referenceErr != nil {
				require.EqualError(t, actualErr, referenceErr.Error())
			} else {
				require.NoError(t, actualErr)
				require.Equal(t, expected, result)
			}
		})
	}
	// Check type predicates and scalar conversions against every JSON value type,
	// including nil and an absent field, which typed getters handle differently.
	for _, function := range []string{"IsString", "IsInt", "IsDouble", "IsBool", "IsMap", "IsList", "Int", "Double", "String"} {
		for _, input := range []string{`{}`, `{"value":null}`, `{"value":"12"}`, `{"value":"not a number"}`, `{"value":12}`, `{"value":1.5}`, `{"value":true}`, `{"value":[]}`, `{"value":{}}`} {
			pattern := `set(attributes["result"], ` + function + `(attributes["value"]))`
			testOTTLMaskMatchesUpstream(t, pattern, input)
		}
	}
	for _, test := range []struct{ pattern, input string }{
		{`set(attributes["credentials"]["password"], "***") where IsMap(attributes["credentials"])`, `{"credentials":{"password":"secret"}}`},
		{`set(attributes["missing"]["password"], "***")`, `{}`},
		{`set(attributes["value"], nil)`, `{"value":"secret"}`},
		{`set(attributes["value"], 9007199254740993)`, `{}`},
		{`set(attributes["value"], attributes["object"])`, `{"object":{}}`},
		{`replace_pattern(body, "(user=)([0-9]+)", "$1***")`, `user=123`},
		{`replace_pattern(attributes["value"], attributes["regex"], "***")`, `{"value":"secret","regex":"^sec"}`},
		{`replace_pattern(attributes["value"], attributes["regex"], "***")`, `{"value":"secret","regex":"["}`},
		{`replace_pattern(body, "[", "***")`, `secret`},
		{`replace_pattern(attributes["value"], "secret", "***")`, `{}`},
		{`replace_pattern(attributes["value"], ".", "***")`, `{"value":42}`},
		{`replace_pattern(body, "secret", "***") where false`, `secret`},
		{`replace_match(body, "sec*", "***")`, `secret`},
		{`replace_match(attributes["value"], attributes["glob"], "***")`, `{"value":"secret","glob":"sec*"}`},
		{`replace_match(attributes["value"], attributes["glob"], "***")`, `{"value":"secret","glob":"["}`},
		{`replace_match(body, "[", "***")`, `secret`},
		{`replace_match(attributes["value"], "*", "***")`, `{"value":false}`},
	} {
		testOTTLMaskMatchesUpstream(t, test.pattern, test.input)
	}
}

func testOTTLMaskMatchesUpstream(t *testing.T, pattern, input string) {
	t.Helper()
	t.Run(pattern+input, func(t *testing.T) {
		parser, err := ottl.NewParser(ottlfuncs.StandardFuncs[*OTTLContext](), parseOTTLMaskPath, component.TelemetrySettings{Logger: zap.NewNop()})
		require.NoError(t, err)
		reference, referenceErr := parser.ParseStatement(pattern)
		actual, actualErr := compileOTTLMaskStatement(pattern)
		if referenceErr != nil {
			require.EqualError(t, actualErr, referenceErr.Error())
			return
		}
		require.NoError(t, actualErr)
		expectedContext := AcquireOTTLContext([]byte(input))
		defer ReleaseOTTLContext(expectedContext)
		actualContext := AcquireOTTLContext([]byte(input))
		defer ReleaseOTTLContext(actualContext)
		expectedChange, referenceErr := expectedContext.ApplyMask(context.Background(), reference)
		actualChange, actualErr := actualContext.ApplyMask(context.Background(), actual)
		if referenceErr != nil {
			require.EqualError(t, actualErr, referenceErr.Error())
		} else {
			require.NoError(t, actualErr)
		}
		require.Equal(t, expectedChange, actualChange)
		require.Equal(t, string(expectedContext.Content()), string(actualContext.Content()))
	})
}
