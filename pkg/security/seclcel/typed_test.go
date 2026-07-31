// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// typedTranslations pairs a SECL expression over the *real* model with the CEL
// it translates to once field types are known.
//
// This is what the type tree buys: the array semantics SECL leaves implicit are
// written out, so an expression means the same thing in both languages instead
// of merely being valid CEL.
var typedTranslations = []struct {
	secl string
	cel  string
}{
	// a scalar field is unaffected
	{`process.file.name == "sh"`, `process.file.name == "sh"`},
	{`exec.file.path =~ "/usr/*"`, `secl.glob(exec.file.path, "/usr/*")`},

	// comparing an iterated field against a scalar means "some element matches"
	{`process.ancestors.file.name == "sh"`, `process.ancestors.exists(elem, elem.file.name == "sh")`},
	{`process.ancestors.uid == 0`, `process.ancestors.exists(elem, elem.uid == 0)`},
	// the negation belongs to the comparison, not to the quantifier
	{`process.ancestors.file.name != "sh"`, `process.ancestors.exists(elem, elem.file.name != "sh")`},

	// membership over an iterated field
	{`process.ancestors.file.name in [ "sh", "bash" ]`,
		`process.ancestors.exists(elem, elem.file.name in ["sh", "bash"])`},
	// `not in` negates the whole quantifier
	{`process.ancestors.file.name not in [ "sh" ]`,
		`!process.ancestors.exists(elem, elem.file.name in ["sh"])`},
	{`process.ancestors.file.name in [ ~"/usr/*" ]`,
		`process.ancestors.exists(elem, secl.glob(elem.file.name, "/usr/*"))`},
	// `allin` asks for every element
	{`process.ancestors.file.name allin [ "sh" ]`,
		`process.ancestors.all(elem, elem.file.name in ["sh"])`},

	// a list valued leaf is quantified in its own right
	{`process.argv == "-l"`, `process.argv.exists(elem, elem == "-l")`},
	{`process.argv in [ "-l", "-a" ]`, `process.argv.exists(elem, elem in ["-l", "-a"])`},
	{`process.argv allin [ "-l" ]`, `process.argv.all(elem, elem in ["-l"])`},

	// a list valued leaf inside an iterated node needs one quantifier per level
	{`process.ancestors.args_flags == "c"`,
		`process.ancestors.exists(elem, elem.args_flags.exists(elem2, elem2 == "c"))`},

	// an iterator variable is already an existential, and correlates the
	// comparisons at one index
	{`process.ancestors[A].file.name == "sh" && process.ancestors[A].uid == 0`,
		`process.ancestors.exists(A, A.file.name == "sh" && A.uid == 0)`},

	// size() applies to the list itself, so this counts the arguments
	{`process.argv.length > 2`, `size(process.argv) > 2`},
	// but for an iterated field it is the length of each element's value
	{`process.ancestors.file.name.length > 3`,
		`process.ancestors.exists(elem, size(elem.file.name) > 3)`},
	{`dns.question.name.root_domain == "example.com"`,
		`secl.rootDomain(dns.question.name) == "example.com"`},

	// only the iterated side is quantified
	{`process.file.name == "sh" && process.ancestors.file.name == "bash"`,
		`process.file.name == "sh" && process.ancestors.exists(elem, elem.file.name == "bash")`},

	{`network.destination.ip in 10.0.0.0/8`,
		`secl.cidrMatch(network.destination.ip, cidr("10.0.0.0/8"))`},
}

func TestTranslateWithTypes(t *testing.T) {
	for _, tt := range typedTranslations {
		t.Run(tt.secl, func(t *testing.T) {
			got, err := TranslateWithTypes(tt.secl, ModelFieldTypes{})
			require.NoError(t, err)
			assert.Equal(t, tt.cel, got)
		})
	}
}

// TestCompileWithTypes checks the translations against the real field types.
// Without the type tree these expressions type-check only because every root is
// declared dynamic; here a wrong type is a failure.
func TestCompileWithTypes(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	for _, tt := range typedTranslations {
		t.Run(tt.secl, func(t *testing.T) {
			checked, err := CompileWithTypes(env, tt.secl, ModelFieldTypes{})
			require.NoError(t, err)
			assert.True(t, checked.IsChecked())
			assert.Equal(t, cel.BoolType, checked.OutputType())
		})
	}
}

// TestModelEnvRejects covers what the type tree lets the checker catch, which
// the dynamic environment used by the untyped tests cannot.
func TestModelEnvRejects(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	tests := []struct {
		expr string
		want string
	}{
		// process.uid is an integer
		{`process.uid == "root"`, "no matching overload"},
		// a misspelt member is named in the error, rather than its root
		{`process.file.nope == "x"`, "undefined field 'nope'"},
		// a field of the SECL unit tests' model, absent from the real one
		{`open.filename == "x"`, "undefined field 'filename'"},
		// selecting through a scalar
		{`process.file.name.deeper == "x"`, "does not support field selection"},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			_, err := CompileWithTypes(env, tt.expr, ModelFieldTypes{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// TestCorpusWithTypes is the broad guard on the array semantics: over the whole
// corpus, the only expressions the real environment rejects must be those naming
// a field the real model does not have, or the documented IPv4 mapped IPv6 case.
// A type error of any other kind means a translation says something SECL does
// not mean.
func TestCorpusWithTypes(t *testing.T) {
	raw, err := os.ReadFile("testdata/corpus.json")
	require.NoError(t, err)

	var corpus []string
	require.NoError(t, json.Unmarshal(raw, &corpus))

	env, err := NewModelEnv()
	require.NoError(t, err)

	var compiled int
	for _, expr := range corpus {
		_, err := CompileWithTypes(env, expr, ModelFieldTypes{})
		if err == nil {
			compiled++
			continue
		}

		msg := err.Error()
		switch {
		// the corpus is mostly written against the SECL unit tests' model
		case strings.Contains(msg, "undefined field"),
			strings.Contains(msg, "undeclared reference"),
			// the network extension rejects these, SECL accepts them
			strings.Contains(msg, "IPv4-mapped IPv6 address"),
			// a name that denotes an object rather than a value, such as a bare
			// `process`. SECL's parser accepts it too and its compiler rejects it.
			strings.Contains(msg, "applied to '(secl."):
			continue
		default:
			t.Errorf("%s\n  translation is not well typed: %s", expr, strings.Split(msg, "\n")[0])
		}
	}

	require.Greater(t, compiled, 250, "expected the real-model expressions to compile")
}
