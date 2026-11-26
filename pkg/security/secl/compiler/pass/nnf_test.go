// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pass

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/parser"
)

func TestNegationNormalForm(t *testing.T) {
	for expr, expected := range map[string]string{
		`open.file.name == "a"`:                                              `(== open.file.name "a")`,
		`!(open.file.name == "a")`:                                           `(!= open.file.name "a")`,
		`!(open.file.name != "a")`:                                           `(== open.file.name "a")`,
		`!(open.file.name =~ "a*")`:                                          `(!~ open.file.name "a*")`,
		`!(open.file.name !~ "a*")`:                                          `(=~ open.file.name "a*")`,
		`!(open.file.name in ["a", "b"])`:                                    `(notin open.file.name ["a" "b"])`,
		`!(open.file.name not in ["a", "b"])`:                                `(in open.file.name ["a" "b"])`,
		`!!(open.file.name == "a")`:                                          `(== open.file.name "a")`,
		`!(open.file.name == "a" && process.uid == 0)`:                       `(|| (!= open.file.name "a") (!= process.uid 0))`,
		`!(open.file.name == "a" || process.uid == 0)`:                       `(&& (!= open.file.name "a") (!= process.uid 0))`,
		`!(open.file.name == "a" && !(process.uid == 0))`:                    `(|| (!= open.file.name "a") (== process.uid 0))`,
		`!(!(open.file.name == "a") || process.is_root)`:                     `(&& (== open.file.name "a") (! process.is_root))`,
		`open.file.name == "a" && !(process.uid == 0)`:                       `(&& (== open.file.name "a") (!= process.uid 0))`,
		`!(open.file.name == "a" && (process.uid == 0 || process.gid == 0))`: `(|| (!= open.file.name "a") (&& (!= process.uid 0) (!= process.gid 0)))`,

		// the leaves without an opposite keep their negation
		`!(process.uid < 10)`:                         `(! (< process.uid 10))`,
		`!process.is_root`:                            `(! process.is_root)`,
		`!(process.argv allin ["a", "b"])`:            `(! (allin process.argv ["a" "b"]))`,
		`!(open.file.name == "a" && process.uid > 0)`: `(|| (!= open.file.name "a") (! (> process.uid 0)))`,
	} {
		t.Run(expr, func(t *testing.T) {
			rule, err := parser.ParseRule(expr)
			require.NoError(t, err)

			normalized, err := Run(rule, ResolveIdents(testLookup), NegationNormalForm())
			require.NoError(t, err)
			require.Equal(t, expected, ast.Sprint(normalized))
		})
	}
}
