// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"sort"
	"testing"

	"github.com/google/cel-go/common/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// TestGlobFieldsAgreeWithSECL is the guard on the generated celGlobFields table.
//
// The generator derives it from the operator override names a field carries, which is a
// list maintained by hand. SECL itself answers the question exactly: compile a rule
// comparing the field against a pattern and read the value type back, which the override
// has rewritten to `glob` if it glob-ifies. So the table is generated and then checked
// against the source of truth, field by field.
func TestGlobFieldsAgreeWithSECL(t *testing.T) {
	var probed int
	var wrong, undecided []string

	for _, field := range readableFields() {
		want, decided := seclCompilesAGlob(field)
		if !decided {
			// A field a pattern cannot be compared against at all has no answer, and
			// no need of one: only a string field ever compiles a matcher.
			if holdsAString(field) && !refusesPatterns[field] {
				undecided = append(undecided, field)
			}
			continue
		}
		probed++

		if _, listed := celGlobFields[field]; listed != want {
			wrong = append(wrong, field)
		}
	}

	sort.Strings(wrong)
	assert.Empty(t, wrong, "celGlobFields disagrees with SECL")

	// Every string field is decided, so the table is checked in both directions
	// rather than only where the probe happened to work.
	sort.Strings(undecided)
	assert.Empty(t, undecided, "SECL would not say what these string fields do with a pattern")

	for field := range refusesPatterns {
		assert.NotContains(t, celGlobFields, field, "a field that refuses a pattern cannot glob one")
	}

	require.Greater(t, probed, 500, "expected the string fields to be probed")
	t.Logf("%d fields probed, %d in the table", probed, len(celGlobFields))
}

// refusesPatterns are the string fields SECL will not compare against a pattern at
// all, so there is no answer to read back. `packet.filter` holds a tcpdump expression
// its override compiles as a whole (PacketFilterMatching), which is why.
//
// They must be absent from the table, which is checked below: a field that refuses a
// pattern cannot be one whose patterns are globs.
var refusesPatterns = map[string]bool{
	"packet.filter": true,
}

// holdsAString reports whether the field, or an element of it, is a string — which is
// the only shape a pattern is ever compared against.
func holdsAString(field string) bool {
	fieldType, _, ok := walkModel(field)
	if !ok {
		return false
	}
	if fieldType.Kind() == types.ListKind {
		fieldType = fieldType.Parameters()[0]
	}
	return fieldType == types.StringType
}

// seclCompilesAGlob reports what SECL makes of a pattern compared against the field, and
// whether it could tell at all: a field that is not a string, or one whose values are
// validated in a way this probe does not satisfy, has no answer here.
func seclCompilesAGlob(field string) (glob bool, decided bool) {
	var m model.Model

	rule, err := eval.NewRule("probe", field+` == ~"/a/*"`, ast.NewParsingContext(false), &eval.Opts{})
	if err != nil {
		return false, false
	}
	if err := rule.GenEvaluator(&m); err != nil {
		return false, false
	}

	for _, value := range rule.GetFieldValues(field) {
		switch value.Type {
		case eval.GlobValueType:
			return true, true
		case eval.PatternValueType:
			return false, true
		}
	}
	return false, false
}

// readableFields is every field the layout has a reader for, sorted so that a failure
// names the same fields in the same order every time.
func readableFields() []string {
	fields := make([]string, 0, len(celReaderIndex))
	for field := range celReaderIndex {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}
