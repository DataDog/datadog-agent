// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package exprlang

import (
	"testing"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		wantExpr  Expr
		wantError bool
	}{
		{
			name:  "valid ref expression",
			input: `{"ref": "s"}`,
			wantExpr: &RefExpr{
				Ref: "s",
			},
		},
		{
			name:  "valid ref with complex variable name",
			input: `{"ref": "myVariable123"}`,
			wantExpr: &RefExpr{
				Ref: "myVariable123",
			},
		},
		{
			name:      "empty ref value",
			input:     `{"ref": ""}`,
			wantError: true,
		},
		{
			name:  "unsupported instruction with string arg",
			input: `{"foo": "bar"}`,
			wantExpr: &UnsupportedExpr{
				operation: "foo",
				argument:  jsontext.Value("bar"),
			},
		},
		{
			name:  "unsupported instruction with number arg",
			input: `{"add": 42}`,
			wantExpr: &UnsupportedExpr{
				operation: "add",
				argument:  jsontext.Value("42"),
			},
		},
		{
			name:  "unsupported instruction with bool arg",
			input: `{"enabled": true}`,
			wantExpr: &UnsupportedExpr{
				operation: "enabled",
				argument:  jsontext.Value(jsontext.True.String()),
			},
		},
		{
			name:  "unsupported instruction with null arg",
			input: `{"value": null}`,
			wantExpr: &UnsupportedExpr{
				operation: "value",
				argument:  nil,
			},
		},
		{
			name:      "empty expression",
			input:     `{}`,
			wantError: true,
		},
		{
			name:      "malformed JSON",
			input:     `{"ref": "}`,
			wantError: true,
		},
		{
			name:      "not an object",
			input:     `"ref"`,
			wantError: true,
		},
		{
			name:      "empty input",
			input:     "",
			wantError: true,
		},
		{
			name:      "ref with non-string value",
			input:     `{"ref": 123}`,
			wantError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := Parse([]byte(tc.input))

			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, expr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantExpr, expr)
			}
		})
	}
}

func TestIsSupported(t *testing.T) {
	testCases := []struct {
		name          string
		expr          Expr
		wantSupported bool
	}{
		{
			name:          "ref expression is supported",
			expr:          &RefExpr{Ref: "s"},
			wantSupported: true,
		},
		{
			name:          "unsupported expression",
			expr:          &UnsupportedExpr{operation: "foo", argument: jsontext.Value("bar")},
			wantSupported: false,
		},
		{
			name:          "nil expression",
			expr:          nil,
			wantSupported: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			supported := IsSupported(tc.expr)
			require.Equal(t, tc.wantSupported, supported)
		})
	}
}

func BenchmarkParse(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "simple_ref",
			input: `{"ref": "s"}`,
		},
		{
			name:  "long_ref",
			input: `{"ref": "thisIsAVeryLongVariableNameThatMightBeUsedInRealWorld"}`,
		},
		{
			name:  "unsupported_string",
			input: `{"add": "someValue"}`,
		},
		{
			name:  "unsupported_number",
			input: `{"multiply": 42}`,
		},
	}

	for _, tc := range testCases {
		input := []byte(tc.input)

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				expr, err := Parse(input)
				if err != nil {
					b.Fatal(err)
				}
				_ = expr
			}
		})
	}
}

// BenchmarkParseParallel tests performance under concurrent load
func BenchmarkParseParallel(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "simple_ref",
			input: `{"ref": "s"}`,
		},
		{
			name:  "unsupported_string",
			input: `{"add": "someValue"}`,
		},
	}

	for _, tc := range testCases {
		input := []byte(tc.input)

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					expr, err := Parse(input)
					if err != nil {
						b.Fatal(err)
					}
					_ = expr
				}
			})
		})
	}
}
