// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package exprlang

import (
	"testing"

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
				Instruction: "foo",
				Argument:    "bar",
			},
		},
		{
			name:  "unsupported instruction with number arg",
			input: `{"add": 42}`,
			wantExpr: &UnsupportedExpr{
				Instruction: "add",
				Argument:    "42",
			},
		},
		{
			name:  "unsupported instruction with bool arg",
			input: `{"enabled": true}`,
			wantExpr: &UnsupportedExpr{
				Instruction: "enabled",
				Argument:    true,
			},
		},
		{
			name:  "unsupported instruction with null arg",
			input: `{"value": null}`,
			wantExpr: &UnsupportedExpr{
				Instruction: "value",
				Argument:    nil,
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
			expr:          &UnsupportedExpr{Instruction: "foo", Argument: "bar"},
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

func TestCollectVariableReferences(t *testing.T) {
	testCases := []struct {
		name     string
		expr     Expr
		wantVars []string
	}{
		{
			name:     "ref expression",
			expr:     &RefExpr{Ref: "myVar"},
			wantVars: []string{"myVar"},
		},
		{
			name:     "unsupported ref expression",
			expr:     &UnsupportedExpr{Instruction: "ref", Argument: "someVar"},
			wantVars: []string{"someVar"},
		},
		{
			name:     "unsupported ref with empty string",
			expr:     &UnsupportedExpr{Instruction: "ref", Argument: ""},
			wantVars: nil,
		},
		{
			name:     "unsupported ref with non-string argument",
			expr:     &UnsupportedExpr{Instruction: "ref", Argument: 123},
			wantVars: nil,
		},
		{
			name:     "non-ref unsupported expression",
			expr:     &UnsupportedExpr{Instruction: "foo", Argument: "bar"},
			wantVars: nil,
		},
		{
			name:     "nil expression",
			expr:     nil,
			wantVars: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vars := CollectVariableReferences(tc.expr)
			require.Equal(t, tc.wantVars, vars)
		})
	}
}
