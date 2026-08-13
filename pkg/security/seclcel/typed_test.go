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

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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
	{`process.file.name == "sh"`, `evt.process.file.name == "sh"`},
	{`exec.file.path =~ "/usr/*"`, `secl.glob(evt.exec.file.path, "/usr/*")`},

	// comparing an iterated field against a scalar means "some element matches"
	{`process.ancestors.file.name == "sh"`, `evt.process.ancestors.exists(elem, elem.file.name == "sh")`},
	{`process.ancestors.uid == 0`, `evt.process.ancestors.exists(elem, elem.uid == 0)`},
	// the negation belongs to the comparison, not to the quantifier
	{`process.ancestors.file.name != "sh"`, `evt.process.ancestors.exists(elem, elem.file.name != "sh")`},

	// membership over an iterated field
	{`process.ancestors.file.name in [ "sh", "bash" ]`,
		`evt.process.ancestors.exists(elem, elem.file.name in ["sh", "bash"])`},
	// `not in` negates the whole quantifier
	{`process.ancestors.file.name not in [ "sh" ]`,
		`!evt.process.ancestors.exists(elem, elem.file.name in ["sh"])`},
	{`process.ancestors.file.name in [ ~"/usr/*" ]`,
		`evt.process.ancestors.exists(elem, secl.glob(elem.file.name, "/usr/*"))`},
	// `allin` asks the same question as `in`, because SECL's compiler does — see
	// TestAllInIsMembershipLikeSECL
	{`process.ancestors.file.name allin [ "sh" ]`,
		`evt.process.ancestors.exists(elem, elem.file.name in ["sh"])`},

	// a list valued leaf is quantified in its own right
	{`process.argv == "-l"`, `evt.process.argv.exists(elem, elem == "-l")`},
	{`process.argv in [ "-l", "-a" ]`, `evt.process.argv.exists(elem, elem in ["-l", "-a"])`},
	{`process.argv allin [ "-l" ]`, `evt.process.argv.exists(elem, elem in ["-l"])`},

	// a list valued leaf inside an iterated node needs one quantifier per level
	{`process.ancestors.args_flags == "c"`,
		`evt.process.ancestors.exists(elem, elem.args_flags.exists(elem2, elem2 == "c"))`},

	// an iterator variable is already an existential, and correlates the
	// comparisons at one index
	{`process.ancestors[A].file.name == "sh" && process.ancestors[A].uid == 0`,
		`evt.process.ancestors.exists(A, A.file.name == "sh" && A.uid == 0)`},

	// size() applies to the list itself, so this counts the arguments
	{`process.argv.length > 2`, `size(evt.process.argv) > 2`},
	// but for an iterated field it is the length of each element's value
	{`process.ancestors.file.name.length > 3`,
		`evt.process.ancestors.exists(elem, size(elem.file.name) > 3)`},
	{`dns.question.name.root_domain == "example.com"`,
		`secl.rootDomain(evt.dns.question.name) == "example.com"`},

	// only the iterated side is quantified
	{`process.file.name == "sh" && process.ancestors.file.name == "bash"`,
		`evt.process.file.name == "sh" && evt.process.ancestors.exists(elem, elem.file.name == "bash")`},

	{`network.destination.ip in 10.0.0.0/8`,
		`secl.cidrMatch(evt.network.destination.ip, cidr("10.0.0.0/8"))`},
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

// TestOnlyAFieldIsRooted is the other half of the rooting. A name the model does
// not know is not a field, so it is left as it was written for the environment to
// resolve, which is how a macro or a constant reaches its declaration — and why
// declaring one can no longer collide with a top level segment.
func TestOnlyAFieldIsRooted(t *testing.T) {
	const expr = `process.file.name in my_macro && process.file.mode == S_IFREG`

	translated, err := TranslateWithTypes(expr, ModelFieldTypes{})
	require.NoError(t, err)
	assert.Equal(t, `secl.matchAny(evt.process.file.name, my_macro) && evt.process.file.mode == S_IFREG`, translated)

	env, err := NewModelEnv(
		cel.Variable("my_macro", cel.ListType(cel.StringType)),
		cel.Variable("S_IFREG", cel.IntType),
	)
	require.NoError(t, err)

	checked, err := CompileWithTypes(env, expr, ModelFieldTypes{})
	require.NoError(t, err)
	assert.True(t, checked.IsChecked())
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
		// a field of the SECL unit tests' model, absent from the real one and not a
		// name it ever had
		{`open.pathname == "x"`, "undefined field 'pathname'"},
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

// TestLegacyFieldNames covers the names a rule may still be written with, which the
// agent maps to the fields that replaced them (eval.go:182) and which thirteen of the
// real rules need.
//
// The mapping is applied where the name is resolved, so what a legacy name reads is the
// current field: the translation shows it, and the value proves it.
func TestLegacyFieldNames(t *testing.T) {
	env, err := NewModelEnv()
	require.NoError(t, err)

	tests := []struct{ secl, cel string }{
		{`open.basename == "passwd"`, `evt.open.file.name == "passwd"`},
		{`open.filename == "/etc/passwd"`, `evt.open.file.path == "/etc/passwd"`},
		{`container.id == "abc"`, `evt.process.container.id == "abc"`},
		// a bare name, which is a field rather than a macro only because the table
		// says so
		{`async == true`, `evt.event.async == true`},
		// the pseudo field suffix is carried across
		{`open.basename.length > 3`, `size(evt.open.file.name) > 3`},
		// and a name that is current already is left alone
		{`open.file.name == "passwd"`, `evt.open.file.name == "passwd"`},
	}

	for _, tt := range tests {
		t.Run(tt.secl, func(t *testing.T) {
			got, err := TranslateWithTypes(tt.secl, ModelFieldTypes{})
			require.NoError(t, err)
			assert.Equal(t, tt.cel, got)
		})
	}

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.Open.File.BasenameStr = "passwd"
	assert.True(t, evalSECL(t, env, event, `open.basename == "passwd"`),
		"a legacy name reads the field that replaced it")
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
		case seclRefuses(expr):
			// A type error the other engine reaches too, so it says something about
			// the expression rather than about the translation. Asking SECL is worth
			// more than matching on the message: a few corpus entries compare a
			// string against an integer, and they only surfaced once the legacy field
			// names started resolving.
			continue
		default:
			t.Errorf("%s\n  translation is not well typed: %s", expr, strings.Split(msg, "\n")[0])
		}
	}

	require.Greater(t, compiled, 250, "expected the real-model expressions to compile")
}

// seclRefuses reports whether SECL's own compiler rejects the expression, against the
// same model and the same constants.
//
// It is how the corpus test tells a translation that says the wrong thing from an
// expression that is simply wrong: only the second is something both engines refuse.
func seclRefuses(expr string) bool {
	var m model.Model

	rule, err := eval.NewRule("corpus", expr, ast.NewParsingContext(false), &eval.Opts{
		Constants:    model.SECLConstants(),
		LegacyFields: model.SECLLegacyFields,
	})
	if err != nil {
		return true
	}
	return rule.GenEvaluator(&m) != nil
}
