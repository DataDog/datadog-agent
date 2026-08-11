// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package powershell

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCommandBasic(t *testing.T) {
	script, err := buildCommand("Get-ClusterNode", "",
		[]parameterEntry{{Name: "Cluster", Value: "PROD-CL01"}},
		nil,
		[]string{"Id", "Name", "NodeWeight"})
	require.NoError(t, err)

	assert.Contains(t, script, "Get-Command -Name 'Get-ClusterNode'")
	assert.Contains(t, script, "-CommandType Cmdlet,Function")
	assert.Contains(t, script, "if (@($c).Count -ne 1)")
	assert.Contains(t, script, "if ($c.Verb -ne 'Get')")
	assert.Contains(t, script, "$p = @{Cluster = 'PROD-CL01'}")
	assert.Contains(t, script, "Select-Object Id,Name,NodeWeight")
	// -InputObject @(...) forces a JSON array for any row count, and it invokes the
	// validated command object ($c) rather than the name.
	assert.Contains(t, script, "ConvertTo-Json -Depth 8 -Compress -InputObject @(& $c @p")
	// No module pinned -> no module check emitted.
	assert.NotContains(t, script, "$c.ModuleName")
}

func TestBuildCommandModuleCheck(t *testing.T) {
	script, err := buildCommand("Get-Service", "Microsoft.PowerShell.Management",
		[]parameterEntry{{Name: "Name", Value: "Dnscache"}}, nil, []string{"Status"})
	require.NoError(t, err)

	// The module pin is enforced at runtime against the resolved command.
	assert.Contains(t, script, "if ($c.ModuleName -ne 'Microsoft.PowerShell.Management')")
	assert.Contains(t, script, "@(& $c @p")
}

func TestBuildCommandModuleWildcardSkipsCheck(t *testing.T) {
	// "*" is the explicit opt-out: no module check is emitted.
	script, err := buildCommand("Get-Service", "*", nil, nil, []string{"Status"})
	require.NoError(t, err)
	assert.NotContains(t, script, "$c.ModuleName")
	assert.Contains(t, script, "@(& $c @p")
}

func TestBuildCommandWherePipeline(t *testing.T) {
	script, err := buildCommand("Get-SmbShare", "SmbShare",
		[]parameterEntry{{Name: "Special", Value: false}},
		[]whereEntry{{Property: "Path", Op: "notlike", Value: "*LocalsplOnly*"}},
		[]string{"Name", "Path"})
	require.NoError(t, err)

	// Where-Object is resolved like the main cmdlet rather than invoked by bare
	// name, so it cannot be shadowed from another module.
	assert.Contains(t, script, "$w = Get-Command -Name 'Where-Object' -CommandType Cmdlet -ErrorAction Stop")
	// One contiguous substring, which also pins the stage order: filtering must
	// precede projection or Select-Object would discard the property being tested.
	assert.Contains(t, script, "| & $w { ($_.Path -notlike '*LocalsplOnly*') } | Select-Object Name,Path")
}

func TestBuildCommandWhereAbsent(t *testing.T) {
	script, err := buildCommand("Get-Service", "", nil, nil, []string{"Status"})
	require.NoError(t, err)
	assert.NotContains(t, script, "Where-Object")
	assert.NotContains(t, script, "$w")
	// The pipeline goes straight from the invocation to the projection.
	assert.Contains(t, script, "@(& $c @p | Select-Object Status)")
}

func TestBuildCommandWhereOnlyPropertyIsNotProjected(t *testing.T) {
	// Path is filtered on but never projected: Where-Object runs first, so a
	// filter-only property does not need to cross the process boundary.
	script, err := buildCommand("Get-SmbShare", "", nil,
		[]whereEntry{{Property: "Path", Op: "notlike", Value: "*x*"}},
		[]string{"Name"})
	require.NoError(t, err)
	assert.Contains(t, script, "Select-Object Name)")
	assert.NotContains(t, script, "Select-Object Name,Path")
}

func TestBuildCommandWhereAndsEntries(t *testing.T) {
	script, err := buildCommand("Get-SmbShare", "", nil,
		[]whereEntry{
			{Property: "Path", Op: "notlike", Value: "*x*"},
			{Property: "Name", Op: "like", Value: "dd*"},
		}, nil)
	require.NoError(t, err)
	assert.Contains(t, script, "{ ($_.Path -notlike '*x*') -and ($_.Name -like 'dd*') }")
}

// A hostile where value must stay inside its single-quoted literal, exactly as a
// parameter value does.
func TestBuildCommandWhereInjectionSafe(t *testing.T) {
	script, err := buildCommand("Get-SmbShare", "", nil,
		[]whereEntry{{Property: "Path", Op: "like", Value: `*'; Remove-Item C:\ -Recurse #`}},
		nil)
	require.NoError(t, err)
	// The single quote is doubled, so the value cannot close the literal.
	assert.Contains(t, script, `($_.Path -like '*''; Remove-Item C:\ -Recurse #')`)
	assert.NotContains(t, script, "'; Remove-Item C:\\ -Recurse #' }")
}

// The core security property: a hostile parameter value must remain inside a
// single-quoted literal and never reach an executable position.
func TestBuildCommandInjectionSafe(t *testing.T) {
	hostile := `PROD-CL01'; Remove-Item C:\ -Recurse #`
	script, err := buildCommand("Get-ClusterNode", "",
		[]parameterEntry{{Name: "Cluster", Value: hostile}},
		nil, nil)
	require.NoError(t, err)

	// The single quote in the value is doubled, keeping it inside the literal.
	assert.Contains(t, script, "Cluster = 'PROD-CL01''; Remove-Item C:\\ -Recurse #'")
	// There must be no bare (unescaped) breakout of the value.
	assert.NotContains(t, script, "'; Remove-Item C:\\ -Recurse #'\n")
}

func TestBuildCommandRejectsBadIdentifiers(t *testing.T) {
	_, err := buildCommand("Get-X", "", []parameterEntry{{Name: "Bad Name", Value: "x"}}, nil, nil)
	assert.Error(t, err)

	_, err = buildCommand("Get-X", "", nil, nil, []string{"Bad Prop"})
	assert.Error(t, err)

	_, err = buildCommand("Remove-Item", "", nil, nil, nil)
	assert.Error(t, err)

	// A where entry naming an invalid property must fail the build too.
	_, err = buildCommand("Get-X", "", nil, []whereEntry{{Property: "Bad Prop", Op: "like", Value: "x"}}, nil)
	assert.Error(t, err)
}

func TestPowershellLiteral(t *testing.T) {
	tests := []struct {
		in   interface{}
		want string
	}{
		{nil, "$null"},
		{true, "$true"},
		{false, "$false"},
		{"plain", "'plain'"},
		{"it's", "'it''s'"},
		{float64(4), "4"},
		{float64(1.5), "1.5"},
		{42, "42"},
	}
	for _, tc := range tests {
		got, err := powershellLiteral(tc.in)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}
}

func TestSingleQuoteDoublesQuotes(t *testing.T) {
	assert.Equal(t, "'a''b'", singleQuote("a'b"))
	assert.False(t, strings.Contains(singleQuote("a'b")[1:len(singleQuote("a'b"))-1], "';"))
}
