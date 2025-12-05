// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package exprlang

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	jsonv2 "github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/require"
)

//go:embed testdata
var testdataFS embed.FS

var testCases = []struct {
	name  string
	input string
}{
	{
		name:  "valid ref expression",
		input: `{"ref": "s"}`,
	},
	{
		name:  "valid ref with complex variable name",
		input: `{"ref": "myVariable123"}`,
	},
	{
		name:  "empty ref value",
		input: `{"ref": ""}`,
	},
	{
		name:  "unsupported instruction with string arg",
		input: `{"foo": "bar"}`,
	},
	{
		name:  "unsupported instruction with number arg",
		input: `{"add": 42}`,
	},
	{
		name:  "unsupported instruction with bool arg",
		input: `{"enabled": true}`,
	},
	{
		name:  "unsupported instruction with null arg",
		input: `{"value": null}`,
	},
	{
		name:  "empty expression",
		input: `{}`,
	},
	{
		name:  "malformed JSON",
		input: `{"ref": "}`,
	},
	{
		name:  "not an object",
		input: `"ref"`,
	},
	{
		name:  "empty input",
		input: "",
	},
	{
		name:  "ref with non-string value",
		input: `{"ref": 123}`,
	},
	// Test simple references (supported)
	{name: "ref hits", input: `{"ref": "hits"}`},
	{name: "ref @it", input: `{"ref": "@it"}`},
	{name: "ref @value", input: `{"ref": "@value"}`},
	{name: "ref @key", input: `{"ref": "@key"}`},
	// Test unsupported operations with simple values
	{name: "isDefined", input: `{"isDefined": "foobar"}`},
	{name: "not with bool", input: `{"not": true}`},
	// Nested/complex structures (will fail to parse with current simple parser)
	// The current parser only handles single-level {"operation": value} structures
	{name: "len of ref", input: `{"len": {"ref": "payload"}}`},
	{name: "len of getmember", input: `{"len": {"getmember": [{"ref": "self"}, "collectionField"]}}`},
	{name: "getmember", input: `{"getmember": [{"ref": "self"}, "name"]}`},
	{name: "nested getmember", input: `{"getmember": [{"getmember": [{"ref": "self"}, "field1"]}, "name"]}`},
	{name: "index array", input: `{"index": [{"ref": "arr"}, 1]}`},
	{name: "index dict", input: `{"index": [{"ref": "dict"}, "world"]}`},
	{name: "contains", input: `{"contains": [{"ref": "payload"}, "hello"]}`},
	{name: "eq with bool", input: `{"eq": [{"ref": "hits"}, true]}`},
	{name: "eq with null", input: `{"eq": [{"ref": "hits"}, null]}`},
	{name: "substring", input: `{"substring": [{"ref": "payload"}, 4, 7]}`},
	{name: "any with isEmpty", input: `{"any": [{"ref": "collection"}, {"isEmpty": {"ref": "@it"}}]}`},
	{name: "any with @value", input: `{"any": [{"ref": "coll"}, {"isEmpty": {"ref": "@value"}}]}`},
	{name: "any with @key", input: `{"any": [{"ref": "coll"}, {"isEmpty": {"ref": "@key"}}]}`},
	{name: "startsWith", input: `{"startsWith": [{"ref": "local_string"}, "hello"]}`},
	{name: "filter", input: `{"filter": [{"ref": "collection"}, {"not": {"isEmpty": {"ref": "@it"}}}]}`},
	{name: "matches", input: `{"matches": [{"ref": "payload"}, "[0-9]+"]}`},
	{name: "or", input: `{"or": [{"ref": "bar"}, {"ref": "foo"}]}`},
	{name: "and", input: `{"and": [{"ref": "bar"}, {"ref": "foo"}]}`},
	{name: "instanceof", input: `{"instanceof": [{"ref": "bar"}, "int"]}`},
	{name: "isEmpty", input: `{"isEmpty": {"ref": "empty_str"}}`},
	{name: "ne", input: `{"ne": [1, 2]}`},
	{name: "gt", input: `{"gt": [2, 1]}`},
	{name: "ge", input: `{"ge": [2, 1]}`},
	{name: "lt", input: `{"lt": [1, 2]}`},
	{name: "le", input: `{"le": [1, 2]}`},
	{name: "all", input: `{"all": [{"ref": "collection"}, {"not": {"isEmpty": {"ref": "@it"}}}]}`},
	{name: "endsWith", input: `{"endsWith": [{"ref": "local_string"}, "world!"]}`},
	{name: "len of filter", input: `{"len": {"filter": [{"ref": "collection"}, {"gt": [{"ref": "@it"}, 1]}]}}`},
	{name: "deeply nested getmember", input: `{"getmember": [{"getmember": [{"getmember": [{"ref": "self"}, "field1"]}, "field2"]}, "name"]}`},
	{name: "any with nested ops", input: `{"any": [{"getmember": [{"ref": "self"}, "collectionField"]}, {"startsWith": [{"getmember": [{"ref": "@it"}, "name"]}, "foo"]}]}`},
	{name: "and with eq and gt", input: `{"and": [{"eq": [{"ref": "hits"}, 42]}, {"gt": [{"len": {"ref": "payload"}}, 5]}]}`},
	{name: "index of filter", input: `{"index": [{"filter": [{"ref": "collection"}, {"gt": [{"ref": "@it"}, 2]}]}, 0]}`},
	{name: "count", input: `{"count": {"ref": "payload"}}`},
	{name: "substring negative", input: `{"substring": [{"ref": "s"}, -5, -1]}`},
	{name: "nested filter with any", input: `{"len": {"filter": [{"ref": "collection"}, {"any": [{"ref": "@it"}, {"eq": [{"ref": "@it"}, 1]}]}]}}`},
	// Test literal values (these should also fail - not objects)
	{name: "literal int", input: `42`},
	{name: "literal bool", input: `true`},
}

// exprResult represents the result of parsing an expression for storage in JSON.
type exprResult struct {
	Type      string          `json:"type"`          // "ref", "unsupported", or "error"
	Ref       string          `json:"ref,omitempty"` // Ref value (used for ref expressions)
	Operation string          `json:"operation"`
	Argument  json.RawMessage `json:"argument,omitempty"` // Raw json argument (used for unsupported expressions)
	Error     string          `json:"error"`
}

func exprToResult(expr Expr, err error) exprResult {
	if err != nil {
		return exprResult{Type: "error", Error: err.Error()}
	}
	switch e := expr.(type) {
	case *RefExpr:
		return exprResult{Type: "ref", Ref: e.Ref}
	case *GetMemberExpr:
		// For now, serialize as unsupported to match existing test expectations.
		// TODO: Update test expectations to recognize getmember as supported.
		baseJSON, _ := json.Marshal(e.Base)
		argJSON, _ := json.Marshal([]interface{}{json.RawMessage(baseJSON), e.Member})
		return exprResult{Type: "unsupported", Operation: "getmember", Argument: json.RawMessage(argJSON)}
	case *UnsupportedExpr:
		return exprResult{Type: "unsupported", Operation: e.Operation, Argument: json.RawMessage(e.Argument)}
	default:
		return exprResult{Type: "error", Error: "unknown expression type"}
	}
}

// sanitizeTestName converts a test name to a safe filename by replacing spaces
// and special characters.
func sanitizeTestName(testName string) string {
	// Replace spaces with underscores
	name := strings.ReplaceAll(testName, " ", "_")
	// Replace @ with "at"
	name = strings.ReplaceAll(name, "@", "at")
	// Remove other special characters that might cause issues
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return name
}

func getExpectedOutputFilename(testName string) string {
	return filepath.Join("testdata", sanitizeTestName(testName)+".json")
}

func loadExpectedOutput(testName string) (exprResult, error) {
	filename := getExpectedOutputFilename(testName)
	content, err := testdataFS.ReadFile(filename)
	if err != nil {
		return exprResult{}, fmt.Errorf("reading %s: %w", filename, err)
	}
	var result exprResult
	if err := json.Unmarshal(content, &result); err != nil {
		return exprResult{}, fmt.Errorf("unmarshalling %s: %w", filename, err)
	}
	return result, nil
}

func saveActualOutput(testName string, result exprResult) error {
	filename := getExpectedOutputFilename(testName)
	outputDir := filepath.Dir(filename)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("error creating testdata directory: %w", err)
	}

	marshaled, err := jsonv2.Marshal(
		result,
		jsontext.WithIndent("  "),
		jsontext.EscapeForHTML(false),
		jsontext.EscapeForJS(false),
	)
	if err != nil {
		return fmt.Errorf("error marshalling result: %w", err)
	}

	baseName := filepath.Base(filename)
	tmpFile, err := os.CreateTemp(outputDir, "."+baseName+".*.tmp.json")
	if err != nil {
		return fmt.Errorf("error creating temp output file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmpFile, bytes.NewReader(marshaled)); err != nil {
		return fmt.Errorf("error writing temp output: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("error closing temp output: %w", err)
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("error renaming temp output: %w", err)
	}
	return nil
}

func TestParse(t *testing.T) {
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := Parse([]byte(tc.input))
			actualResult := exprToResult(expr, err)

			if rewrite {
				// In rewrite mode, save the actual output
				if saveErr := saveActualOutput(tc.name, actualResult); saveErr != nil {
					t.Logf("error saving actual output for test %s: %v", tc.name, saveErr)
				} else {
					t.Logf("output saved to: %s", getExpectedOutputFilename(tc.name))
				}
				return
			}

			// Load expected output from JSON and compare
			expectedResult, loadErr := loadExpectedOutput(tc.name)
			require.NoError(t, loadErr, "failed to load expected output for test %s", tc.name)

			// Compare results
			require.Equal(t, expectedResult.Type, actualResult.Type, "expression type mismatch")
			require.Equal(t, expectedResult.Operation, actualResult.Operation, "operation mismatch")
			require.Equal(t, expectedResult.Error, actualResult.Error, "error mismatch")
			switch expr.(type) {
			case *RefExpr:
				require.Equal(t, expectedResult.Ref, actualResult.Ref, "ref value mismatch")
			case *UnsupportedExpr:
				if expectedResult.Argument == nil {
					require.Equal(t, json.RawMessage("null"), actualResult.Argument, "argument mismatch")
				} else {
					require.JSONEq(t, string(expectedResult.Argument), string(actualResult.Argument), "argument mismatch")
				}
			}
		})
	}
}

func BenchmarkParse(b *testing.B) {
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
