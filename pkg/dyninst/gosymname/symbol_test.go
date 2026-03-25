// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// YAML golden test infrastructure
// ---------------------------------------------------------------------------

var rewriteFromEnv = func() bool {
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	return rewrite
}()
var rewrite = flag.Bool("rewrite", rewriteFromEnv, "rewrite the golden test files")

// testCaseInput is what we read from the YAML file on rewrite — just the
// inputs. This lets us change the output format without breaking the rewrite
// path.
type testCaseInput struct {
	Name  string    `yaml:"name,omitempty"`
	Input testInput `yaml:"input"`
}

type testInput struct {
	Symbol string `yaml:"symbol"`
	Source string `yaml:"source,omitempty"` // defaults to "dwarf"
}

// testCase is the full structure with both input and output.
type testCase struct {
	Name   string     `yaml:"name,omitempty"`
	Input  testInput  `yaml:"input"`
	Output testOutput `yaml:"output"`
}

type testOutput struct {
	Pkg     string       `yaml:"pkg,omitempty"`
	Local   string       `yaml:"local"`
	Class   string       `yaml:"class"`
	Interps []testInterp `yaml:"interps"`
}

type testInterp struct {
	QualifiedName string   `yaml:"qualified_name"`
	Receiver      string   `yaml:"receiver,omitempty"`
	Function      string   `yaml:"function"`
	Inlined       []string `yaml:"inlined,omitempty"`
	Closure       string   `yaml:"closure,omitempty"`
	Depth         int      `yaml:"depth,omitempty"`
	Wrapper       string   `yaml:"wrapper,omitempty"`
	ABI           string   `yaml:"abi,omitempty"`
}

func parseSource(s string) SymbolSource {
	switch s {
	case "pclntab":
		return SourcePclntab
	case "nm":
		return SourceNM
	case "pprof":
		return SourcePprof
	case "", "dwarf":
		return SourceDWARF
	default:
		panic(fmt.Sprintf("unknown source %q", s))
	}
}

func formatClassName(c SymbolClass) string {
	switch c {
	case ClassFunction:
		return "function"
	case ClassClosure:
		return "closure"
	case ClassInit:
		return "init"
	case ClassMapInit:
		return "map_init"
	case ClassGlobalClosure:
		return "global_closure"
	case ClassCompilerGenerated:
		return "compiler_generated"
	case ClassBareName:
		return "bare_name"
	case ClassCFunction:
		return "c_function"
	default:
		return fmt.Sprintf("unknown(%d)", c)
	}
}

func formatWrapperKind(w WrapperKind) string {
	switch w {
	case WrapperNone:
		return ""
	case WrapperGoWrap:
		return "gowrap"
	case WrapperDeferWrap:
		return "deferwrap"
	case WrapperMethodExpr:
		return "method_expr"
	default:
		return fmt.Sprintf("unknown(%d)", w)
	}
}

// formatReceiver formats a receiver name with its kind and optional generics.
func formatReceiver(name string, kind ReceiverKind, gen *GenericParams) string {
	if kind == ReceiverNone {
		return ""
	}
	var s string
	if kind == ReceiverPointer {
		s = "*" + name
	} else {
		s = name
	}
	if gen != nil {
		s += "[" + gen.Raw + "]"
	}
	return s
}

// formatFuncWithGenerics formats a function name with optional generics.
func formatFuncWithGenerics(name string, gen *GenericParams) string {
	if gen != nil {
		return name + "[" + gen.Raw + "]"
	}
	return name
}

// formatInlinedCall formats a single InlinedCall as a compact string.
func formatInlinedCall(ic *InlinedCall) string {
	var s string
	if ic.HasReceiver {
		s = "*" + ic.Receiver
		if ic.ReceiverGenerics != nil {
			s += "[" + ic.ReceiverGenerics.Raw + "]"
		}
		s += "." + ic.Function
	} else {
		s = ic.Function
	}
	if ic.FuncGenerics != nil {
		s += "[" + ic.FuncGenerics.Raw + "]"
	}
	return s
}

func formatTestInterp(interp *Interpretation) testInterp {
	ti := testInterp{
		QualifiedName: interp.QualifiedName(),
		Receiver:      formatReceiver(interp.OuterReceiver, interp.OuterReceiverKind, interp.OuterReceiverGenerics),
		Function:      formatFuncWithGenerics(interp.OuterFunction, interp.OuterFuncGenerics),
		Closure:       interp.ClosureSuffix,
		Depth:         interp.ClosureDepth,
		Wrapper:       formatWrapperKind(interp.Wrapper),
		ABI:           interp.ABISuffix,
	}
	for i := range interp.InlinedCalls {
		ti.Inlined = append(ti.Inlined, formatInlinedCall(&interp.InlinedCalls[i]))
	}
	return ti
}

func formatTestOutput(s *Symbol) testOutput {
	interps := s.Interpretations()
	out := testOutput{
		Pkg:   s.Package(),
		Local: s.Local(),
		Class: formatClassName(s.Class()),
	}
	for i := range interps {
		out.Interps = append(out.Interps, formatTestInterp(&interps[i]))
	}
	return out
}

// ---------------------------------------------------------------------------
// Golden test
// ---------------------------------------------------------------------------

func TestSymbols(t *testing.T) {
	const path = "testdata/symbols.yaml"
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	if *rewrite {
		var inputs []testCaseInput
		require.NoError(t, yaml.Unmarshal(raw, &inputs))
		var cases []testCase
		for _, in := range inputs {
			source := parseSource(in.Input.Source)
			s := Parse(in.Input.Symbol, source)
			cases = append(cases, testCase{
				Name:   in.Name,
				Input:  in.Input,
				Output: formatTestOutput(&s),
			})
		}
		out, err := yaml.MarshalWithOptions(cases, yaml.IndentSequence(true))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, out, 0644))
		t.Logf("rewrote %s with %d cases", path, len(cases))
		return
	}

	var cases []testCase
	require.NoError(t, yaml.Unmarshal(raw, &cases))
	for _, tc := range cases {
		name := tc.Name
		if name == "" {
			name = tc.Input.Symbol
		}
		t.Run(name, func(t *testing.T) {
			source := parseSource(tc.Input.Source)
			s := Parse(tc.Input.Symbol, source)
			got := formatTestOutput(&s)
			assert.Equal(t, tc.Output, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Non-golden tests: these test specific APIs beyond Parse+Interpretations
// ---------------------------------------------------------------------------

func TestSplitPackage(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		source SymbolSource
		pkg    string
		local  string
	}{
		{"simple", "encoding/json.Marshal", SourcePclntab, "encoding/json", "Marshal"},
		{"escaped", "gopkg.in/square/go-jose%2ev2.newBuffer", SourceDWARF, "gopkg.in/square/go-jose.v2", "newBuffer"},
		{"bare", "indexbytebody", SourceNM, "", "indexbytebody"},
		{"runtime", "runtime.gcMarkDone", SourcePclntab, "runtime", "gcMarkDone"},
		{"malformed_escape", "example%ZZ.Function", SourceDWARF, "example%ZZ", "Function"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, local := SplitPackage(tt.input, tt.source)
			assert.Equal(t, tt.pkg, pkg)
			assert.Equal(t, tt.local, local)
		})
	}
}

func TestCachingBehavior(t *testing.T) {
	s := Parse("encoding/json.Marshal", SourcePclntab)
	interps1 := s.Interpretations()
	interps2 := s.Interpretations()
	assert.Equal(t, len(interps1), len(interps2))
	assert.Equal(t, interps1[0], interps2[0])
}

func TestParseInto(t *testing.T) {
	var s Symbol
	ParseInto(&s, "encoding/json.Marshal", SourcePclntab)
	assert.Equal(t, "encoding/json", s.Package())
	assert.Equal(t, "Marshal", s.Local())
}

func TestIsGeneric(t *testing.T) {
	s := Parse("pkg.(*bucket[go.shape.uint64,go.shape.*uint8]).add", SourceDWARF)
	assert.True(t, s.IsGeneric())
}
