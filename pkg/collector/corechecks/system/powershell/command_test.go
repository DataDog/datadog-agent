// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package powershell

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCommandBasic(t *testing.T) {
	script, err := buildCommand("Get-ClusterNode", "",
		[]filterEntry{{Name: "Cluster", Value: "PROD-CL01"}},
		[]string{"Id", "Name", "NodeWeight"})
	require.NoError(t, err)

	assert.Contains(t, script, "Get-Command -Name 'Get-ClusterNode'")
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
		[]filterEntry{{Name: "Name", Value: "Dnscache"}}, []string{"Status"})
	require.NoError(t, err)

	// The module pin is enforced at runtime against the resolved command.
	assert.Contains(t, script, "if ($c.ModuleName -ne 'Microsoft.PowerShell.Management')")
	assert.Contains(t, script, "@(& $c @p")
}

func TestBuildCommandModuleWildcardSkipsCheck(t *testing.T) {
	// "*" is the explicit opt-out: no module check is emitted.
	script, err := buildCommand("Get-Service", "*", nil, []string{"Status"})
	require.NoError(t, err)
	assert.NotContains(t, script, "$c.ModuleName")
	assert.Contains(t, script, "@(& $c @p")
}

// The core security property: a hostile parameter value must remain inside a
// single-quoted literal and never reach an executable position.
func TestBuildCommandInjectionSafe(t *testing.T) {
	hostile := `PROD-CL01'; Remove-Item C:\ -Recurse #`
	script, err := buildCommand("Get-ClusterNode", "",
		[]filterEntry{{Name: "Cluster", Value: hostile}},
		nil)
	require.NoError(t, err)

	// The single quote in the value is doubled, keeping it inside the literal.
	assert.Contains(t, script, "Cluster = 'PROD-CL01''; Remove-Item C:\\ -Recurse #'")
	// There must be no bare (unescaped) breakout of the value.
	assert.NotContains(t, script, "'; Remove-Item C:\\ -Recurse #'\n")
}

func TestBuildCommandRejectsBadIdentifiers(t *testing.T) {
	_, err := buildCommand("Get-X", "", []filterEntry{{Name: "Bad Name", Value: "x"}}, nil)
	assert.Error(t, err)

	_, err = buildCommand("Get-X", "", nil, []string{"Bad Prop"})
	assert.Error(t, err)

	_, err = buildCommand("Remove-Item", "", nil, nil)
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
