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
	// Test literal values
	{name: "literal int", input: `42`},
	{name: "literal bool", input: `true`},
	// Test eq expressions
	{name: "eq with int", input: `{"eq": [{"ref": "x"}, 42]}`},
	{name: "eq with string", input: `{"eq": [{"ref": "name"}, "hello"]}`},
	{name: "eq with float", input: `{"eq": [{"ref": "x"}, 3.14]}`},
	{name: "literal string", input: `"hello"`},
	{name: "literal float", input: `3.14`},
	{name: "literal false", input: `false`},
	{name: "literal null", input: `null`},
	// Test len/isEmpty inside eq
	{name: "eq with len", input: `{"eq": [{"len": {"ref": "s"}}, 5]}`},
	{name: "eq with isEmpty", input: `{"eq": [{"isEmpty": {"ref": "s"}}, true]}`},
}

// exprResult represents the result of parsing an expression for storage in JSON.
type exprResult struct {
	Type      string          `json:"type"`          // "ref", "unsupported", "eq", "literal", or "error"
	Ref       string          `json:"ref,omitempty"` // Ref value (used for ref expressions)
	Operation string          `json:"operation"`
	Argument  json.RawMessage `json:"argument,omitempty"` // Raw json argument (used for unsupported expressions)
	Left      *exprResult     `json:"left,omitempty"`     // Left operand (used for eq expressions)
	Right     *exprResult     `json:"right,omitempty"`    // Right operand (used for eq expressions)
	Value     any             `json:"value,omitempty"`    // Literal value
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
	case *LenExpr:
		operand := exprToResult(e.Operand, nil)
		return exprResult{Type: "len", Left: &operand}
	case *IsEmptyExpr:
		operand := exprToResult(e.Operand, nil)
		return exprResult{Type: "isEmpty", Left: &operand}
	case *EqExpr:
		left := exprToResult(e.Left, nil)
		right := exprToResult(e.Right, nil)
		return exprResult{Type: "eq", Left: &left, Right: &right}
	case *IndexExpr:
		base := exprToResult(e.Base, nil)
		index := exprToResult(e.Index, nil)
		return exprResult{Type: "index", Left: &base, Right: &index}
	case *LiteralExpr:
		return exprResult{Type: "literal", Value: e.Value}
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
			case *LenExpr, *IsEmptyExpr:
				actualJSON, _ := json.Marshal(actualResult)
				expectedJSON, _ := json.Marshal(expectedResult)
				require.JSONEq(t, string(expectedJSON), string(actualJSON), "len/isEmpty expression mismatch")
			case *IndexExpr:
				actualJSON, _ := json.Marshal(actualResult)
				expectedJSON, _ := json.Marshal(expectedResult)
				require.JSONEq(t, string(expectedJSON), string(actualJSON), "index expression mismatch")
			case *EqExpr:
				actualJSON, _ := json.Marshal(actualResult)
				expectedJSON, _ := json.Marshal(expectedResult)
				require.JSONEq(t, string(expectedJSON), string(actualJSON), "eq expression mismatch")
			case *LiteralExpr:
				actualJSON, _ := json.Marshal(actualResult)
				expectedJSON, _ := json.Marshal(expectedResult)
				require.JSONEq(t, string(expectedJSON), string(actualJSON), "literal expression mismatch")
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

// TestRewrite exhaustively covers the Rewrite function across all expression
// types, confirming that:
//   - leaves (RefExpr, LiteralExpr, UnsupportedExpr) are visited and can be
//     replaced or left alone,
//   - every branch node (GetMemberExpr, EqExpr, IndexExpr, LenExpr,
//     IsEmptyExpr) recurses into each child and rebuilds only when a child
//     changed,
//   - the visit order is bottom-up (children visited before parents),
//   - unhandled expression types cause a panic.
func TestRewrite(t *testing.T) {
	t.Run("nil rewriter preserves identity", func(t *testing.T) {
		cases := []Expr{
			&RefExpr{Ref: "x"},
			&LiteralExpr{Value: int64(42)},
			&UnsupportedExpr{Operation: "foo", Argument: nil},
			&GetMemberExpr{Base: &RefExpr{Ref: "x"}, Member: "f"},
			&EqExpr{Left: &RefExpr{Ref: "a"}, Right: &LiteralExpr{Value: int64(1)}},
			&IndexExpr{Base: &RefExpr{Ref: "a"}, Index: &LiteralExpr{Value: int64(0)}},
			&LenExpr{Operand: &RefExpr{Ref: "s"}},
			&IsEmptyExpr{Operand: &RefExpr{Ref: "s"}},
		}
		for _, in := range cases {
			out := Rewrite(in, func(Expr) Expr { return nil })
			require.Samef(t, in, out, "expected identity for %T", in)
		}
	})

	t.Run("rewrites RefExpr leaf", func(t *testing.T) {
		in := &RefExpr{Ref: "x"}
		out := Rewrite(in, func(e Expr) Expr {
			if ref, ok := e.(*RefExpr); ok && ref.Ref == "x" {
				return &RefExpr{Ref: "y"}
			}
			return nil
		})
		require.Equal(t, &RefExpr{Ref: "y"}, out)
	})

	t.Run("rewrites LiteralExpr leaf", func(t *testing.T) {
		in := &LiteralExpr{Value: int64(1)}
		out := Rewrite(in, func(e Expr) Expr {
			if _, ok := e.(*LiteralExpr); ok {
				return &LiteralExpr{Value: int64(2)}
			}
			return nil
		})
		require.Equal(t, &LiteralExpr{Value: int64(2)}, out)
	})

	t.Run("rewrites UnsupportedExpr leaf", func(t *testing.T) {
		in := &UnsupportedExpr{Operation: "foo"}
		out := Rewrite(in, func(e Expr) Expr {
			if _, ok := e.(*UnsupportedExpr); ok {
				return &RefExpr{Ref: "replaced"}
			}
			return nil
		})
		require.Equal(t, &RefExpr{Ref: "replaced"}, out)
	})

	t.Run("rewrites GetMemberExpr base, preserves member", func(t *testing.T) {
		in := &GetMemberExpr{Base: &RefExpr{Ref: "a"}, Member: "f"}
		out := Rewrite(in, func(e Expr) Expr {
			if ref, ok := e.(*RefExpr); ok && ref.Ref == "a" {
				return &RefExpr{Ref: "b"}
			}
			return nil
		})
		require.Equal(t, &GetMemberExpr{Base: &RefExpr{Ref: "b"}, Member: "f"}, out)
	})

	t.Run("rewrites both sides of EqExpr", func(t *testing.T) {
		in := &EqExpr{Left: &RefExpr{Ref: "a"}, Right: &RefExpr{Ref: "b"}}
		out := Rewrite(in, func(e Expr) Expr {
			if ref, ok := e.(*RefExpr); ok {
				return &RefExpr{Ref: ref.Ref + "!"}
			}
			return nil
		})
		require.Equal(t, &EqExpr{
			Left:  &RefExpr{Ref: "a!"},
			Right: &RefExpr{Ref: "b!"},
		}, out)
	})

	t.Run("rewrites both sides of IndexExpr", func(t *testing.T) {
		in := &IndexExpr{Base: &RefExpr{Ref: "a"}, Index: &LiteralExpr{Value: int64(0)}}
		out := Rewrite(in, func(e Expr) Expr {
			switch e := e.(type) {
			case *RefExpr:
				return &RefExpr{Ref: "b"}
			case *LiteralExpr:
				if v, ok := e.Value.(int64); ok {
					return &LiteralExpr{Value: v + 1}
				}
			}
			return nil
		})
		require.Equal(t, &IndexExpr{
			Base:  &RefExpr{Ref: "b"},
			Index: &LiteralExpr{Value: int64(1)},
		}, out)
	})

	t.Run("rewrites LenExpr operand", func(t *testing.T) {
		in := &LenExpr{Operand: &RefExpr{Ref: "s"}}
		out := Rewrite(in, func(e Expr) Expr {
			if ref, ok := e.(*RefExpr); ok && ref.Ref == "s" {
				return &RefExpr{Ref: "t"}
			}
			return nil
		})
		require.Equal(t, &LenExpr{Operand: &RefExpr{Ref: "t"}}, out)
	})

	t.Run("rewrites IsEmptyExpr operand", func(t *testing.T) {
		in := &IsEmptyExpr{Operand: &RefExpr{Ref: "s"}}
		out := Rewrite(in, func(e Expr) Expr {
			if ref, ok := e.(*RefExpr); ok && ref.Ref == "s" {
				return &RefExpr{Ref: "t"}
			}
			return nil
		})
		require.Equal(t, &IsEmptyExpr{Operand: &RefExpr{Ref: "t"}}, out)
	})

	t.Run("preserves identity when children unchanged", func(t *testing.T) {
		cases := []Expr{
			&GetMemberExpr{Base: &RefExpr{Ref: "x"}, Member: "f"},
			&EqExpr{Left: &RefExpr{Ref: "a"}, Right: &RefExpr{Ref: "b"}},
			&IndexExpr{Base: &RefExpr{Ref: "a"}, Index: &LiteralExpr{Value: int64(0)}},
			&LenExpr{Operand: &RefExpr{Ref: "s"}},
			&IsEmptyExpr{Operand: &RefExpr{Ref: "s"}},
		}
		for _, in := range cases {
			out := Rewrite(in, func(Expr) Expr { return nil })
			require.Samef(t, in, out,
				"unchanged subtree should not reallocate %T", in)
		}
	})

	t.Run("rebuilds parent only when child changed", func(t *testing.T) {
		base := &RefExpr{Ref: "a"}
		in := &GetMemberExpr{Base: base, Member: "f"}
		out := Rewrite(in, func(e Expr) Expr {
			if ref, ok := e.(*RefExpr); ok && ref.Ref == "a" {
				return &RefExpr{Ref: "b"}
			}
			return nil
		}).(*GetMemberExpr)
		require.NotSame(t, in, out)
		require.NotSame(t, base, out.Base)
		require.Equal(t, "f", out.Member)
	})

	t.Run("bottom-up visit order", func(t *testing.T) {
		// Expression: len(a.f)
		// Expected visit order: a (leaf), a.f (parent), len(...) (root)
		in := &LenExpr{Operand: &GetMemberExpr{
			Base:   &RefExpr{Ref: "a"},
			Member: "f",
		}}
		var order []string
		Rewrite(in, func(e Expr) Expr {
			switch e := e.(type) {
			case *RefExpr:
				order = append(order, "ref:"+e.Ref)
			case *GetMemberExpr:
				order = append(order, "getmember:"+e.Member)
			case *LenExpr:
				order = append(order, "len")
			}
			return nil
		})
		require.Equal(t, []string{"ref:a", "getmember:f", "len"}, order)
	})

	t.Run("nested rewrite propagates through tree", func(t *testing.T) {
		// eq(len(a.f), 3) -> eq(len(b.f), 3)
		in := &EqExpr{
			Left: &LenExpr{Operand: &GetMemberExpr{
				Base:   &RefExpr{Ref: "a"},
				Member: "f",
			}},
			Right: &LiteralExpr{Value: int64(3)},
		}
		out := Rewrite(in, func(e Expr) Expr {
			if ref, ok := e.(*RefExpr); ok && ref.Ref == "a" {
				return &RefExpr{Ref: "b"}
			}
			return nil
		})
		require.Equal(t, &EqExpr{
			Left: &LenExpr{Operand: &GetMemberExpr{
				Base:   &RefExpr{Ref: "b"},
				Member: "f",
			}},
			Right: &LiteralExpr{Value: int64(3)},
		}, out)
	})

	t.Run("replacement at root replaces entire tree", func(t *testing.T) {
		in := &EqExpr{
			Left:  &RefExpr{Ref: "a"},
			Right: &LiteralExpr{Value: int64(1)},
		}
		out := Rewrite(in, func(e Expr) Expr {
			if _, ok := e.(*EqExpr); ok {
				return &LiteralExpr{Value: true}
			}
			return nil
		})
		require.Equal(t, &LiteralExpr{Value: true}, out)
	})

	t.Run("panics on unhandled expression type", func(t *testing.T) {
		require.PanicsWithValue(t,
			fmt.Sprintf("exprlang.Rewrite: unhandled expression type %T",
				(*unknownExpr)(nil)),
			func() {
				Rewrite(&unknownExpr{}, func(Expr) Expr { return nil })
			})
	})
}

// unknownExpr is a test-only Expr type that the Rewrite switch does not
// recognise; used to exercise the default panic.
type unknownExpr struct{}

// TestChildren covers the Children iterator: it visits every node bottom-up
// (leaves before parents, root last), handles all node types, and supports
// early termination via break.
func TestChildren(t *testing.T) {
	collect := func(root Expr) []Expr {
		var out []Expr
		for e := range Children(root) {
			out = append(out, e)
		}
		return out
	}

	t.Run("yields single leaf", func(t *testing.T) {
		in := &RefExpr{Ref: "x"}
		require.Equal(t, []Expr{in}, collect(in))
	})

	t.Run("bottom-up traversal", func(t *testing.T) {
		// eq(len(a.f), 3)
		refA := &RefExpr{Ref: "a"}
		gm := &GetMemberExpr{Base: refA, Member: "f"}
		l := &LenExpr{Operand: gm}
		lit := &LiteralExpr{Value: int64(3)}
		root := &EqExpr{Left: l, Right: lit}

		got := collect(root)
		require.Equal(t, []Expr{refA, gm, l, lit, root}, got)
	})

	t.Run("visits IndexExpr base and index", func(t *testing.T) {
		base := &RefExpr{Ref: "a"}
		idx := &LiteralExpr{Value: int64(0)}
		root := &IndexExpr{Base: base, Index: idx}
		require.Equal(t, []Expr{base, idx, root}, collect(root))
	})

	t.Run("visits IsEmptyExpr operand", func(t *testing.T) {
		op := &RefExpr{Ref: "s"}
		root := &IsEmptyExpr{Operand: op}
		require.Equal(t, []Expr{op, root}, collect(root))
	})

	t.Run("early break stops iteration", func(t *testing.T) {
		// eq(ref(a), ref(b)) — break after first node.
		in := &EqExpr{Left: &RefExpr{Ref: "a"}, Right: &RefExpr{Ref: "b"}}
		var count int
		for range Children(in) {
			count++
			break
		}
		require.Equal(t, 1, count)
	})

	t.Run("break after match skips remainder", func(t *testing.T) {
		in := &EqExpr{Left: &RefExpr{Ref: "target"}, Right: &RefExpr{Ref: "other"}}
		var visited []string
		for e := range Children(in) {
			if ref, ok := e.(*RefExpr); ok {
				visited = append(visited, ref.Ref)
				if ref.Ref == "target" {
					break
				}
			}
		}
		require.Equal(t, []string{"target"}, visited)
	})
}

func (*unknownExpr) expr() {}
