// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package powershell

import (
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

// Every emitted form must be total: comparing a missing, null, or wrongly-typed
// property has to drop the row rather than throw, because the script runs under
// $ErrorActionPreference = 'Stop' and a throw would fail the whole check run.
func TestWhereConditionForms(t *testing.T) {
	tests := []struct {
		entry whereEntry
		want  string
	}{
		// String operators: PowerShell stringifies the left-hand side already.
		{whereEntry{Property: "Path", Op: "notlike", Value: "*Locals*"}, `($_.Path -notlike '*Locals*')`},
		{whereEntry{Property: "Path", Op: "like", Value: "*Microsoft*"}, `($_.Path -like '*Microsoft*')`},
		{whereEntry{Property: "Name", Op: "match", Value: "^dd"}, `($_.Name -match '^dd')`},
		{whereEntry{Property: "Name", Op: "notmatch", Value: "^dd"}, `($_.Name -notmatch '^dd')`},
		// Equality casts explicitly, so a DateTime property compared against a
		// non-date string yields false instead of throwing.
		{whereEntry{Property: "State", Op: "eq", Value: "Running"}, `([string]$_.State -eq 'Running')`},
		{whereEntry{Property: "State", Op: "ne", Value: "Running"}, `([string]$_.State -ne 'Running')`},
		// Ordering coerces with -as [double] and guards against $null, because bare
		// "$null -lt 1024" is TRUE in PowerShell.
		{whereEntry{Property: "Length", Op: "gt", Value: 1024}, `(($_.Length -as [double]) -ne $null -and ($_.Length -as [double]) -gt 1024)`},
		{whereEntry{Property: "Length", Op: "le", Value: 0}, `(($_.Length -as [double]) -ne $null -and ($_.Length -as [double]) -le 0)`},
	}
	for _, tc := range tests {
		got, err := tc.entry.condition()
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, "op %s", tc.entry.Op)
	}
}

func TestWhereClauseJoinsWithAnd(t *testing.T) {
	clause, err := whereClause([]whereEntry{
		{Property: "Path", Op: "notlike", Value: "*x*"},
		{Property: "Name", Op: "like", Value: "dd*"},
	})
	require.NoError(t, err)
	assert.Equal(t, `($_.Path -notlike '*x*') -and ($_.Name -like 'dd*')`, clause)

	// No entries means no Where-Object stage at all.
	clause, err = whereClause(nil)
	require.NoError(t, err)
	assert.Equal(t, "", clause)
}

// The security-relevant case: a quote in the value must be doubled so it stays
// inside the literal.
func TestWhereQuoteEscaping(t *testing.T) {
	got, err := (&whereEntry{Property: "Name", Op: "like", Value: "O'Brien*"}).condition()
	require.NoError(t, err)
	assert.Equal(t, `($_.Name -like 'O''Brien*')`, got)
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

func TestWhereRejectsNonNumericOrderingValue(t *testing.T) {
	// An ordering comparison against a non-numeric value could never match any
	// row, so it is a configuration error rather than a silent no-data condition.
	_, err := parseInstanceConfig([]byte(
		"cmdlet: Get-SmbShare\nmetrics:\n  - [1, share]\nwhere:\n  - [Length, gt, abc]\n"))
	assert.Error(t, err)

	// A numeric string is still fine: toFloat accepts it, as it does for metrics.
	inst, err := parseInstanceConfig([]byte(
		"cmdlet: Get-SmbShare\nmetrics:\n  - [1, share]\nwhere:\n  - [Length, gt, '1024']\n"))
	require.NoError(t, err)
	require.Len(t, inst.Where, 1)
}

func TestWhereRejectsBadPropertyIdentifier(t *testing.T) {
	// The property name is emitted into the command, so it must be a safe
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
