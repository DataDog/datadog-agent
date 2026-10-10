// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package powershell

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWherePositional(t *testing.T) {
	data := []byte(`
cmdlet: Get-SmbShare
metrics:
  - [1, share]
where:
  - [Path, notlike, '*LocalsplOnly*']
  - [Length, gt, 1024]
`)
	inst, err := parseInstanceConfig(data)
	require.NoError(t, err)

	require.Len(t, inst.Where, 2)
	assert.Equal(t, "Path", inst.Where[0].Property)
	assert.Equal(t, "notlike", inst.Where[0].Op)
	assert.Equal(t, "*LocalsplOnly*", inst.Where[0].Value)
	assert.Equal(t, "Length", inst.Where[1].Property)
	assert.Equal(t, "gt", inst.Where[1].Op)
}

// Operators are accepted in any casing. Both layers must agree, and schema
// validation runs first in Configure, so exercise them in that order.
func TestWhereOperatorCasingAccepted(t *testing.T) {
	base := "cmdlet: Get-Service\nmetrics:\n  - [1, s]\nwhere:\n"
	forms := map[string]string{
		"tuple":   base + "  - [Status, %s, Running]\n",
		"mapping": base + "  - property: Status\n    op: %s\n    value: Running\n",
	}
	for form, tmpl := range forms {
		for _, op := range []string{"eq", "EQ", "Eq", "notlike", "NotLike", "NOTLIKE", "' gt '"} {
			data := []byte(fmt.Sprintf(tmpl, op))
			require.NoError(t, validateInstanceSchema(data), "%s form, op %s: schema", form, op)
			inst, err := parseInstanceConfig(data)
			require.NoError(t, err, "%s form, op %s: parse", form, op)
			require.Len(t, inst.Where, 1)
			assert.Equal(t, normalizeOp(strings.Trim(op, "'")), inst.Where[0].Op)
		}
	}
}

// Normalization must not widen the accepted set: an unknown operator still fails.
func TestWhereOperatorCasingDoesNotWidenTheSet(t *testing.T) {
	base := "cmdlet: Get-Service\nmetrics:\n  - [1, s]\nwhere:\n"
	for _, op := range []string{"contains", "CONTAINS", "totallybogus"} {
		mapping := []byte(base + "  - property: Status\n    op: " + op + "\n    value: x\n")
		assert.Error(t, validateInstanceSchema(mapping), "mapping form, op %s", op)

		// The tuple form's schema does not constrain the operator, so finalize is
		// what rejects it there.
		tuple := []byte(base + "  - [Status, " + op + ", x]\n")
		_, err := parseInstanceConfig(tuple)
		assert.Error(t, err, "tuple form, op %s", op)
	}
}

func TestParseWhereMappingForm(t *testing.T) {
	data := []byte(`
cmdlet: Get-SmbShare
metrics:
  - [1, share]
where:
  - property: Path
    op: NotLike
    value: '*x*'
`)
	inst, err := parseInstanceConfig(data)
	require.NoError(t, err)
	require.Len(t, inst.Where, 1)
	assert.Equal(t, "Path", inst.Where[0].Property)
	assert.Equal(t, "notlike", inst.Where[0].Op) // normalized to lower case
}

// Every operator emits the same shape, so comparison semantics are PowerShell's
// own. Property and value are read from $cfg; only the switch varies with config.
func TestWhereStageForms(t *testing.T) {
	const refs = "$wp0 = @{ Property = $cfg.where[0].property; Value = $cfg.where[0].value; "

	tests := []struct {
		entry  whereEntry
		wantSw string
	}{
		{whereEntry{Property: "Path", Op: "notlike", Value: "*Locals*"}, "NotLike"},
		{whereEntry{Property: "Path", Op: "like", Value: "*Microsoft*"}, "Like"},
		{whereEntry{Property: "Name", Op: "match", Value: "^dd"}, "Match"},
		{whereEntry{Property: "Name", Op: "notmatch", Value: "^dd"}, "NotMatch"},
		{whereEntry{Property: "State", Op: "eq", Value: "Running"}, "EQ"},
		{whereEntry{Property: "State", Op: "ne", Value: "Running"}, "NE"},
		{whereEntry{Property: "Length", Op: "gt", Value: 1024}, "GT"},
		{whereEntry{Property: "Length", Op: "ge", Value: 1024}, "GE"},
		{whereEntry{Property: "Length", Op: "lt", Value: 1024}, "LT"},
		{whereEntry{Property: "Length", Op: "le", Value: 0}, "LE"},
	}
	for _, tc := range tests {
		var decls, pipe strings.Builder
		require.NoError(t, tc.entry.writeStage(&decls, &pipe, 0))
		assert.Equal(t, refs+tc.wantSw+" = $true }\n", decls.String(), "op %s", tc.entry.Op)
		assert.Equal(t, " | & $w @wp0", pipe.String(), "op %s", tc.entry.Op)
	}
}

// Stage variables are suffixed with the entry index so two stages cannot collide.
func TestWhereStageIsIndexed(t *testing.T) {
	var decls, pipe strings.Builder
	require.NoError(t, (&whereEntry{Property: "Name", Op: "like", Value: "dd*"}).writeStage(&decls, &pipe, 3))
	assert.Equal(t, "$wp3 = @{ Property = $cfg.where[3].property; Value = $cfg.where[3].value; Like = $true }\n", decls.String())
	assert.Equal(t, " | & $w @wp3", pipe.String())
}

// schema.go duplicates this operator list, so pin the size and require every entry
// to name a Where-Object switch.
func TestWhereOpsTableIsConsistent(t *testing.T) {
	require.Len(t, whereOps, 10)
	for name, sw := range whereOps {
		assert.NotEmpty(t, sw, "op %q needs a Where-Object switch", name)
	}
}

// A hostile value must not appear in the emitted stage at all; it reaches the child
// only through the payload.
func TestWhereValueNeverEntersTheStage(t *testing.T) {
	for _, value := range []string{
		"O'Brien*",
		"x\u2019 -or $(New-Item -Path C:/dd_test.txt) -or \u2019y",
		"x\u2018 + $(New-Item -Path C:/dd_test.txt) + \u2018y",
	} {
		var decls, pipe strings.Builder
		require.NoError(t, (&whereEntry{Property: "Name", Op: "like", Value: value}).writeStage(&decls, &pipe, 0))
		assert.NotContains(t, decls.String(), value)
		assert.NotContains(t, pipe.String(), value)
	}
}

func TestWhereRejectsMalformedTuple(t *testing.T) {
	base := "cmdlet: Get-SmbShare\nmetrics:\n  - [1, share]\n"

	// A 2-element tuple is not a valid where entry.
	_, err := parseInstanceConfig([]byte(base + "where:\n  - [Path, notlike]\n"))
	assert.Error(t, err)

	// Neither is a 4-element one.
	_, err = parseInstanceConfig([]byte(base + "where:\n  - [Path, notlike, a, b]\n"))
	assert.Error(t, err)
}

func TestWhereRejectsUnsupportedOperator(t *testing.T) {
	_, err := parseInstanceConfig([]byte(
		"cmdlet: Get-SmbShare\nmetrics:\n  - [1, share]\nwhere:\n  - [Path, contains, x]\n"))
	assert.Error(t, err)
}

func TestWhereRejectsNonScalarValue(t *testing.T) {
	base := "cmdlet: Get-SmbShare\nmetrics:\n  - [1, share]\n"

	_, err := parseInstanceConfig([]byte(base + "where:\n  - property: Path\n    op: like\n    value: [a, b]\n"))
	assert.Error(t, err)

	_, err = parseInstanceConfig([]byte(base + "where:\n  - property: Path\n    op: like\n    value: {k: v}\n"))
	assert.Error(t, err)
}

// An explicit null is the supported way to filter on an unset property, and is
// what the schema's required `value` leaves available. See schema.go.
func TestWhereAcceptsExplicitNullValue(t *testing.T) {
	inst, err := parseInstanceConfig([]byte(
		"cmdlet: Get-SmbShare\nmetrics:\n  - [1, share]\nwhere:\n  - property: Path\n    op: eq\n    value: null\n"))
	require.NoError(t, err)
	require.Len(t, inst.Where, 1)
	assert.Nil(t, inst.Where[0].Value)
}

// Ordering operators take any scalar, because PowerShell decides the comparison
// from the property's type rather than from the configured value.
func TestWhereOrderingAcceptsAnyScalar(t *testing.T) {
	for _, value := range []string{"1024", "'1024'", "abc", "1.5"} {
		inst, err := parseInstanceConfig([]byte(
			"cmdlet: Get-SmbShare\nmetrics:\n  - [1, share]\nwhere:\n  - [Length, gt, " + value + "]\n"))
		require.NoError(t, err, "value %s", value)
		require.Len(t, inst.Where, 1)
	}
}

func TestWhereRejectsBadPropertyIdentifier(t *testing.T) {
	// The property name is bound to Where-Object -Property, so it must be a plain
	// identifier — no dots, spaces, or expression syntax.
	_, err := parseInstanceConfig([]byte(
		"cmdlet: Get-SmbShare\nmetrics:\n  - [1, share]\nwhere:\n  - ['Path.Sub', like, x]\n"))
	assert.Error(t, err)

	_, err = parseInstanceConfig([]byte(
		"cmdlet: Get-SmbShare\nmetrics:\n  - [1, share]\nwhere:\n  - ['$(whoami)', like, x]\n"))
	assert.Error(t, err)
}

func TestWhereDoesNotAffectSelectProperties(t *testing.T) {
	// Filtering happens before projection, so a where property is deliberately
	// absent from the Select-Object list.
	inst := &instanceConfig{
		Metrics: []metricEntry{{Property: "1", Name: "share"}},
		TagBy:   []tagByEntry{{Property: "Name", Alias: "share_name"}},
		Where:   []whereEntry{{Property: "Path", Op: "notlike", Value: "*x*"}},
	}
	assert.ElementsMatch(t, []string{"Name"}, inst.selectProperties())
}
