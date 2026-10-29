// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pass

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/parser"
)

func TestProjectField(t *testing.T) {
	subscript := regexp.MustCompile(`\[[^\]]*\]`)
	fieldOf := func(name string) (string, bool) {
		switch name {
		case "true", "false", "O_CREAT":
			return "", false
		}
		return subscript.ReplaceAllString(name, ""), true
	}

	for _, test := range []struct {
		expr     string
		field    string
		expected string
	}{
		{`open.file.path == "/a" && process.uid == 0`, "open.file.path", `(== open.file.path "/a")`},
		{`open.file.path == "/a" && process.uid == 0`, "process.uid", `(== process.uid 0)`},
		{`open.file.path == "/a" || process.uid == 0`, "open.file.path", `true`},
		{`process.uid == 0`, "open.file.path", `true`},

		// an unknown leaf under a negation is false
		{`!(open.file.path == "/a" && process.uid == 0)`, "open.file.path", `true`},
		{`!(open.file.path == "/a" || process.uid == 0)`, "open.file.path", `(! (== open.file.path "/a"))`},
		{`!!(open.file.path == "/a" && process.uid == 0)`, "open.file.path", `(! (! (== open.file.path "/a")))`},
		{`!process.is_kworker && open.file.name == "a"`, "open.file.name", `(== open.file.name "a")`},

		// a leaf is kept or replaced as a whole
		{`open.file.path == process.file.path && open.flags & O_CREAT > 0`, "open.flags", `(> (& open.flags O_CREAT) 0)`},
		{`open.file.path == process.file.path`, "open.file.path", `true`},
		{`open.file.path in ["/a", ~"/b/*"] && process.name not in ["sh"]`, "process.name", `(notin process.name ["sh"])`},
		{`true && open.file.name == "a"`, "open.file.name", `(&& true (== open.file.name "a"))`},

		// values resolved at evaluation
		{`open.file.path == "${var}" && process.uid == 0`, "open.file.path", `true`},
		{`open.file.path == "/%{process.comm}" && process.uid == 0`, "open.file.path", `true`},
		{`open.file.path in ${paths}`, "open.file.path", `true`},

		// iterators
		{`process.ancestors[A].file.name == "sh" && process.uid == 0`, "process.ancestors.file.name", `(== process.ancestors[A].file.name "sh")`},
	} {
		t.Run(test.expr+" on "+test.field, func(t *testing.T) {
			rule, err := parser.ParseRule(test.expr)
			require.NoError(t, err)
			original := ast.Sprint(rule)

			projected := Run(rule, ProjectField(test.field, fieldOf))
			require.Equal(t, test.expected, ast.Sprint(projected))
			require.Equal(t, original, ast.Sprint(rule), "the parsed tree is left unchanged")
		})
	}
}

func TestProjectFieldMacros(t *testing.T) {
	macro, err := parser.ParseMacro("is_root", `process.uid == 0 && open.file.path == "/a"`)
	require.NoError(t, err)
	lookup := func(name string) (*ast.Macro, bool) {
		return macro, name == "is_root"
	}
	fieldOf := func(name string) (string, bool) { return name, true }

	rule, err := parser.ParseRule(`is_root && process.name == "sh"`)
	require.NoError(t, err)

	projected := Run(rule, ExpandMacros(lookup), ProjectField("open.file.path", fieldOf))
	require.Equal(t, `{is_root (== open.file.path "/a")}`, ast.Sprint(projected))
}
