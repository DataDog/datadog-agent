// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package powershell

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// quoteBreakoutValues are values that would close a single-quoted string, which
// PowerShell does on U+0027 and on U+2018, U+2019, U+201A and U+201B alike.
var quoteBreakoutValues = []struct {
	name  string
	value string
}{
	{"ascii U+0027", `x'; New-Item -Path C:/dd_test.txt #`},
	{"U+2018", "x\u2018 + $(New-Item -Path C:/dd_test.txt) + \u2018y"},
	{"U+2019", "x\u2019 -or $(New-Item -Path C:/dd_test.txt) -or \u2019y"},
	{"U+201A", "x\u201a + $(New-Item -Path C:/dd_test.txt) + \u201ay"},
	{"U+201B", "x\u201b + $(New-Item -Path C:/dd_test.txt) + \u201by"},
	{"mixed opener and closer", "x\u2018 + $(New-Item -Path C:/dd_test.txt) + \u2019y"},
	{"backtick and newline", "x`'; New-Item -Path C:/dd_test.txt\n#"},
}

func unmarshalPayload(t *testing.T, data []byte) scriptPayload {
	t.Helper()
	var p scriptPayload
	require.NoError(t, json.Unmarshal(data, &p))
	return p
}

// The generated script must depend only on a config's shape, never on the values in
// it. That is what keeps a hostile value structurally unable to reach a code position.
func TestScriptIsValueIndependent(t *testing.T) {
	build := func(t *testing.T, v interface{}) (string, scriptPayload) {
		t.Helper()
		script, data, err := buildCommand("Get-Service", "Microsoft.PowerShell.Management",
			[]parameterEntry{{Name: "Name", Value: v}},
			[]whereEntry{{Property: "Status", Op: "eq", Value: v}},
			[]string{"Name", "Status"})
		require.NoError(t, err)
		return script, unmarshalPayload(t, data)
	}

	baseline, _ := build(t, "Running")

	for _, tc := range quoteBreakoutValues {
		t.Run(tc.name, func(t *testing.T) {
			script, payload := build(t, tc.value)

			// Byte-identical to a benign config of the same shape.
			assert.Equal(t, baseline, script)
			// Its own assertion: invariance would also hold if every value were
			// interpolated identically badly.
			assert.NotContains(t, script, tc.value)
			// The value must still reach the child, verbatim, as data.
			require.Len(t, payload.Parameters, 1)
			assert.Equal(t, tc.value, payload.Parameters[0].Value)
			require.Len(t, payload.Where, 1)
			assert.Equal(t, tc.value, payload.Where[0].Value)
		})
	}
}

// The cmdlet name, module and projected property names are configuration too, so
// they must not appear in the script either.
func TestScriptIsIndependentOfNamesAndModule(t *testing.T) {
	scriptA, _, err := buildCommand("Get-Service", "Microsoft.PowerShell.Management",
		[]parameterEntry{{Name: "Name", Value: "x"}}, nil, []string{"Status"})
	require.NoError(t, err)
	scriptB, _, err := buildCommand("Get-SmbShare", "SmbShare",
		[]parameterEntry{{Name: "Special", Value: "x"}}, nil, []string{"Path"})
	require.NoError(t, err)

	assert.Equal(t, scriptA, scriptB, "same shape, different names: script should be identical")
	for _, s := range []string{"Get-Service", "Get-SmbShare", "SmbShare", "Microsoft.PowerShell.Management", "Status", "Path", "Special"} {
		assert.NotContains(t, scriptA, s)
		assert.NotContains(t, scriptB, s)
	}
}

// exec.Command escapes the script with syscall.EscapeArg, which turns an embedded
// double quote into \" before PowerShell's own parser sees it.
func TestScriptContainsNoDoubleQuote(t *testing.T) {
	script, _, err := buildCommand("Get-Service", "Microsoft.PowerShell.Management",
		[]parameterEntry{{Name: "Name", Value: "x"}},
		[]whereEntry{
			{Property: "Status", Op: "eq", Value: "Running"},
			{Property: "Length", Op: "gt", Value: 1024},
		},
		[]string{"Name"})
	require.NoError(t, err)
	assert.NotContains(t, script, `"`)
}

func TestBuildCommandScriptShape(t *testing.T) {
	script, _, err := buildCommand("Get-ClusterNode", "",
		[]parameterEntry{{Name: "Cluster", Value: "PROD-CL01"}},
		nil,
		[]string{"Id", "Name", "NodeWeight"})
	require.NoError(t, err)

	// The payload is read from stdin to EOF, decoded as UTF-8 explicitly.
	assert.Contains(t, script, "[Console]::OpenStandardInput()")
	assert.Contains(t, script, "Text.UTF8Encoding($false)")
	assert.Contains(t, script, "ConvertFrom-Json")
	// And the result JSON is written back as UTF-8, or non-ASCII values in metric
	// values and tags are corrupted by the OEM codepage.
	assert.Contains(t, script, "[Console]::OutputEncoding = New-Object Text.UTF8Encoding($false)")
	// The cmdlet is re-resolved and re-checked at runtime, by value not by name.
	assert.Contains(t, script, "Get-Command -Name $cfg.cmdlet")
	assert.Contains(t, script, "-CommandType Cmdlet,Function")
	assert.Contains(t, script, "if (@($c).Count -ne 1)")
	assert.Contains(t, script, "if ($c.Verb -ne 'Get')")
	// Parameters are splatted from the payload, and projection reads the payload.
	assert.Contains(t, script, "foreach ($e in $cfg.parameters) { $p[$e.name] = $e.value }")
	assert.Contains(t, script, "| Select-Object -Property $cfg.select")
	// -InputObject @(...) forces a JSON array for any row count, and it invokes the
	// validated command object ($c) rather than the name.
	assert.Contains(t, script, "ConvertTo-Json -Depth 8 -Compress -InputObject @(& $c @p")
	// No module pinned -> no module check emitted.
	assert.NotContains(t, script, "$c.ModuleName")
}

func TestBuildCommandModuleCheck(t *testing.T) {
	script, _, err := buildCommand("Get-Service", "Microsoft.PowerShell.Management",
		[]parameterEntry{{Name: "Name", Value: "Dnscache"}}, nil, []string{"Status"})
	require.NoError(t, err)

	// The module pin is enforced at runtime against the resolved command, comparing
	// against the payload rather than an embedded name.
	assert.Contains(t, script, "if ($c.ModuleName -ne $cfg.module)")
	assert.Contains(t, script, "@(& $c @p")
}

func TestBuildCommandModuleWildcardSkipsCheck(t *testing.T) {
	// "*" is the explicit opt-out: no module check is emitted. Whether the guard
	// exists stays a Go-side decision rather than a branch in the script.
	script, _, err := buildCommand("Get-Service", "*", nil, nil, []string{"Status"})
	require.NoError(t, err)
	assert.NotContains(t, script, "$c.ModuleName")
	assert.Contains(t, script, "@(& $c @p")
}

func TestBuildCommandWherePipeline(t *testing.T) {
	script, _, err := buildCommand("Get-SmbShare", "SmbShare",
		[]parameterEntry{{Name: "Special", Value: false}},
		[]whereEntry{{Property: "Path", Op: "notlike", Value: "*LocalsplOnly*"}},
		[]string{"Name", "Path"})
	require.NoError(t, err)

	// Where-Object is resolved like the main cmdlet rather than invoked by bare
	// name, so it cannot be shadowed from another module.
	assert.Contains(t, script, "$w = Get-Command -Name 'Where-Object' -CommandType Cmdlet -ErrorAction Stop")
	// Property and value are read from the payload and bound as data.
	assert.Contains(t, script, "$wp0 = @{ Property = $cfg.where[0].property; Value = $cfg.where[0].value; NotLike = $true }")
	assert.Contains(t, script, "| & $w @wp0")

	// Filtering must precede projection. Asserted by position, not as one contiguous
	// substring, so it survives unrelated whitespace changes.
	assert.Less(t, strings.Index(script, "| & $w @wp0"), strings.Index(script, "| Select-Object"))
}

func TestBuildCommandWhereAbsent(t *testing.T) {
	script, _, err := buildCommand("Get-Service", "", nil, nil, []string{"Status"})
	require.NoError(t, err)
	assert.NotContains(t, script, "Where-Object")
	assert.NotContains(t, script, "$w = Get-Command")
	// The pipeline goes straight from the invocation to the projection.
	assert.Contains(t, script, "@(& $c @p | Select-Object -Property $cfg.select)")
}

func TestBuildCommandNoSelectPropertiesOmitsProjection(t *testing.T) {
	// Select-Object -Property with an empty list throws, and selectProperties
	// legitimately returns nothing when every metric is virtual.
	script, _, err := buildCommand("Get-Service", "", nil, nil, nil)
	require.NoError(t, err)
	assert.NotContains(t, script, "Select-Object")
	assert.Contains(t, script, "@(& $c @p)")
}

func TestBuildCommandWhereOnlyPropertyIsNotProjected(t *testing.T) {
	// Path is filtered on but never projected: Where-Object runs first, so a
	// filter-only property does not need to cross the process boundary.
	script, data, err := buildCommand("Get-SmbShare", "", nil,
		[]whereEntry{{Property: "Path", Op: "notlike", Value: "*x*"}},
		[]string{"Name"})
	require.NoError(t, err)
	assert.Contains(t, script, "| & $w @wp0")

	payload := unmarshalPayload(t, data)
	assert.Equal(t, []string{"Name"}, payload.Select)
	require.Len(t, payload.Where, 1)
	assert.Equal(t, "Path", payload.Where[0].Property)
}

// Each entry becomes its own Where-Object stage rather than one -and-joined
// scriptblock. That is equivalent for conjunction: a row dropped by an earlier
// stage is never retested by a later one.
func TestBuildCommandWhereChainsOneStagePerEntry(t *testing.T) {
	script, data, err := buildCommand("Get-SmbShare", "", nil,
		[]whereEntry{
			{Property: "Path", Op: "notlike", Value: "*x*"},
			{Property: "Name", Op: "like", Value: "dd*"},
		}, nil)
	require.NoError(t, err)

	assert.Contains(t, script, "$wp0 = @{ Property = $cfg.where[0].property; Value = $cfg.where[0].value; NotLike = $true }")
	assert.Contains(t, script, "$wp1 = @{ Property = $cfg.where[1].property; Value = $cfg.where[1].value; Like = $true }")
	assert.Contains(t, script, "| & $w @wp0 | & $w @wp1")
	// Stage order follows config order.
	assert.Less(t, strings.Index(script, "@wp0"), strings.Index(script, "| & $w @wp1"))

	payload := unmarshalPayload(t, data)
	require.Len(t, payload.Where, 2)
	assert.Equal(t, "Path", payload.Where[0].Property)
	assert.Equal(t, "Name", payload.Where[1].Property)
}

// Ordering operators emit the same -Property/-Value form as the rest, so the
// comparison is PowerShell's own rather than one reimplemented here.
func TestBuildCommandNumericWhereStage(t *testing.T) {
	script, _, err := buildCommand("Get-ChildItem", "", nil,
		[]whereEntry{{Property: "Length", Op: "gt", Value: 1024}}, []string{"Name"})
	require.NoError(t, err)

	assert.Contains(t, script, "$wp0 = @{ Property = $cfg.where[0].property; Value = $cfg.where[0].value; GT = $true }")
	assert.Contains(t, script, "| & $w @wp0")
	// No scriptblock stage, so nothing is evaluated per row.
	assert.NotContains(t, script, "& $w {")
}

func TestPayloadCarriesScalarsVerbatim(t *testing.T) {
	_, data, err := buildCommand("Get-SmbShare", "SmbShare",
		[]parameterEntry{
			{Name: "Special", Value: false},
			{Name: "Count", Value: 42},
			{Name: "Ratio", Value: 1.5},
			{Name: "Missing", Value: nil},
			{Name: "Text", Value: "plain"},
		}, nil, nil)
	require.NoError(t, err)

	payload := unmarshalPayload(t, data)
	require.Len(t, payload.Parameters, 5)
	assert.Equal(t, false, payload.Parameters[0].Value)
	assert.Equal(t, float64(42), payload.Parameters[1].Value) // JSON numbers decode as float64
	assert.Equal(t, 1.5, payload.Parameters[2].Value)
	assert.Nil(t, payload.Parameters[3].Value)
	assert.Equal(t, "plain", payload.Parameters[4].Value)
}

// The generated script reads these exact lower-case names. PowerShell member
// access is case-insensitive, so a missing json tag would work by accident and
// leave the script/payload contract implicit.
func TestPayloadKeyNames(t *testing.T) {
	_, data, err := buildCommand("Get-Service", "M",
		[]parameterEntry{{Name: "Name", Value: "x"}},
		[]whereEntry{{Property: "Status", Op: "eq", Value: "Running"}},
		[]string{"Name"})
	require.NoError(t, err)

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))
	for _, k := range []string{"cmdlet", "module", "parameters", "where", "select"} {
		assert.Contains(t, top, k)
	}

	var params []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["parameters"], &params))
	require.Len(t, params, 1)
	assert.Contains(t, params[0], "name")
	assert.Contains(t, params[0], "value")

	var wheres []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["where"], &wheres))
	require.Len(t, wheres, 1)
	assert.Contains(t, wheres[0], "property")
	assert.Contains(t, wheres[0], "value")
	// The operator is fixed into the script from the closed whereOps map and must
	// never travel in the payload.
	assert.NotContains(t, wheres[0], "op")
}

// Empty collections are normalized so the payload shape does not depend on whether
// Go handed us a nil slice.
func TestPayloadNormalizesEmptyCollections(t *testing.T) {
	_, data, err := buildCommand("Get-Service", "*", nil, nil, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"cmdlet":"Get-Service","module":"*","parameters":[],"where":[],"select":[]}`, string(data))
}

func TestBuildCommandRejectsBadIdentifiers(t *testing.T) {
	_, _, err := buildCommand("Get-X", "", []parameterEntry{{Name: "Bad Name", Value: "x"}}, nil, nil)
	assert.Error(t, err)

	_, _, err = buildCommand("Get-X", "", nil, nil, []string{"Bad Prop"})
	assert.Error(t, err)

	_, _, err = buildCommand("Remove-Item", "", nil, nil, nil)
	assert.Error(t, err)

	// buildPayload re-checks the where property, so a hand-constructed entry cannot
	// bypass finalize.
	_, _, err = buildCommand("Get-X", "", nil, []whereEntry{{Property: "Bad Prop", Op: "like", Value: "x"}}, nil)
	assert.Error(t, err)
}

// PowerShell hashtable keys are case-insensitive, so the splat loop would silently
// overwrite a duplicate.
func TestBuildCommandRejectsDuplicateParameter(t *testing.T) {
	_, _, err := buildCommand("Get-Service", "", []parameterEntry{
		{Name: "Name", Value: "a"},
		{Name: "name", Value: "b"},
	}, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than once")
}

func TestBuildCommandRejectsTooManyWhereEntries(t *testing.T) {
	where := make([]whereEntry, maxWhereEntries+1)
	for i := range where {
		where[i] = whereEntry{Property: "Status", Op: "eq", Value: "Running"}
	}
	_, _, err := buildCommand("Get-Service", "", nil, where, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many")
}
