// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package powershell

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleAllowlist = `
version: 1
allowed_cmdlets:
  Get-ClusterNode:
    module: FailoverClusters
    parameters:
      Cluster: { required: false, allowed_values: [PROD-CL01, PROD-CL02] }
  Get-ClusterGroup:
    module: FailoverClusters
  Get-Certificate:
    module: PKI
    parameters:
      Template: { required: false, pattern: '^[A-Za-z0-9 _.-]+$' }
`

func mustAllowlist(t *testing.T) *allowlist {
	t.Helper()
	al, err := parseAllowlist([]byte(sampleAllowlist))
	require.NoError(t, err)
	return al
}

func TestParseAllowlistErrors(t *testing.T) {
	_, err := parseAllowlist([]byte(""))
	assert.Error(t, err)

	_, err = parseAllowlist([]byte("version: 2\nallowed_cmdlets:\n  Get-X: {}\n"))
	assert.Error(t, err)

	_, err = parseAllowlist([]byte("version: 1\nallowed_cmdlets: {}\n"))
	assert.Error(t, err)

	// a non-Get cmdlet in the allowlist is rejected
	_, err = parseAllowlist([]byte("version: 1\nallowed_cmdlets:\n  Remove-Item: {}\n"))
	assert.Error(t, err)
}

func TestValidateInstanceAccepts(t *testing.T) {
	al := mustAllowlist(t)
	inst := &instanceConfig{
		Cmdlet:  "Get-ClusterNode",
		Filters: []filterEntry{{Name: "Cluster", Value: "PROD-CL01"}},
		TagQueries: []tagQueryEntry{
			{LinkSourceProperty: "Id", TargetCmdlet: "Get-ClusterGroup", LinkTargetProperty: "OwnerNode", TargetProperty: "Name"},
		},
	}
	assert.NoError(t, al.validateInstance(inst))
}

func TestValidateInstanceRejectsUnlistedCmdlet(t *testing.T) {
	al := mustAllowlist(t)
	err := al.validateInstance(&instanceConfig{Cmdlet: "Get-Process"})
	assert.Error(t, err)
}

func TestValidateInstanceRejectsUndeclaredParam(t *testing.T) {
	al := mustAllowlist(t)
	err := al.validateInstance(&instanceConfig{
		Cmdlet:  "Get-ClusterNode",
		Filters: []filterEntry{{Name: "Name", Value: "x"}},
	})
	assert.Error(t, err)
}

func TestValidateInstanceRejectsValueNotAllowed(t *testing.T) {
	al := mustAllowlist(t)
	err := al.validateInstance(&instanceConfig{
		Cmdlet:  "Get-ClusterNode",
		Filters: []filterEntry{{Name: "Cluster", Value: "EVIL-CL"}},
	})
	assert.Error(t, err)
}

func TestValidateInstanceRejectsPatternMismatch(t *testing.T) {
	al := mustAllowlist(t)
	err := al.validateInstance(&instanceConfig{
		Cmdlet:  "Get-Certificate",
		Filters: []filterEntry{{Name: "Template", Value: "bad;value"}},
	})
	assert.Error(t, err)

	assert.NoError(t, al.validateInstance(&instanceConfig{
		Cmdlet:  "Get-Certificate",
		Filters: []filterEntry{{Name: "Template", Value: "datadog agent"}},
	}))
}

func TestValidateInstanceRejectsUnlistedTagQueryCmdlet(t *testing.T) {
	al := mustAllowlist(t)
	err := al.validateInstance(&instanceConfig{
		Cmdlet: "Get-ClusterNode",
		TagQueries: []tagQueryEntry{
			{LinkSourceProperty: "Id", TargetCmdlet: "Get-Secret", LinkTargetProperty: "Y", TargetProperty: "Z"},
		},
	})
	assert.Error(t, err)
}

func TestValidateInstanceRequiredParam(t *testing.T) {
	al, err := parseAllowlist([]byte(`
version: 1
allowed_cmdlets:
  Get-Thing:
    module: ThingModule
    parameters:
      Scope: { required: true }
`))
	require.NoError(t, err)

	err = al.validateInstance(&instanceConfig{Cmdlet: "Get-Thing"})
	assert.Error(t, err) // missing required param

	assert.NoError(t, al.validateInstance(&instanceConfig{
		Cmdlet:  "Get-Thing",
		Filters: []filterEntry{{Name: "Scope", Value: "all"}},
	}))
}

func TestParseAllowlistRequiresModule(t *testing.T) {
	// A cmdlet entry without a module is rejected (strict, secure-by-default).
	_, err := parseAllowlist([]byte("version: 1\nallowed_cmdlets:\n  Get-Service:\n    parameters:\n      Name: {}\n"))
	assert.Error(t, err)

	// "*" is the explicit opt-out and is accepted.
	_, err = parseAllowlist([]byte("version: 1\nallowed_cmdlets:\n  Get-Service:\n    module: \"*\"\n"))
	assert.NoError(t, err)
}
