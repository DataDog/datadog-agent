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
		Cmdlet:     "Get-ClusterNode",
		Parameters: []parameterEntry{{Name: "Cluster", Value: "PROD-CL01"}},
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
		Cmdlet:     "Get-ClusterNode",
		Parameters: []parameterEntry{{Name: "Name", Value: "x"}},
	})
	assert.Error(t, err)
}

func TestValidateInstanceRejectsValueNotAllowed(t *testing.T) {
	al := mustAllowlist(t)
	err := al.validateInstance(&instanceConfig{
		Cmdlet:     "Get-ClusterNode",
		Parameters: []parameterEntry{{Name: "Cluster", Value: "EVIL-CL"}},
	})
	assert.Error(t, err)
}

func TestValidateInstanceRejectsPatternMismatch(t *testing.T) {
	al := mustAllowlist(t)
	err := al.validateInstance(&instanceConfig{
		Cmdlet:     "Get-Certificate",
		Parameters: []parameterEntry{{Name: "Template", Value: "bad;value"}},
	})
	assert.Error(t, err)

	assert.NoError(t, al.validateInstance(&instanceConfig{
		Cmdlet:     "Get-Certificate",
		Parameters: []parameterEntry{{Name: "Template", Value: "datadog agent"}},
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
      Scope: { required: true, allowed_values: [all, local] }
`))
	require.NoError(t, err)

	err = al.validateInstance(&instanceConfig{Cmdlet: "Get-Thing"})
	assert.Error(t, err) // missing required param

	assert.NoError(t, al.validateInstance(&instanceConfig{
		Cmdlet:     "Get-Thing",
		Parameters: []parameterEntry{{Name: "Scope", Value: "all"}},
	}))
}

func TestParseAllowlistRequiresValueConstraint(t *testing.T) {
	// Every declared parameter must set allowed_values or pattern; a parameter
	// with neither is rejected at load time (fail closed).
	_, err := parseAllowlist([]byte(`
version: 1
allowed_cmdlets:
  Get-Thing:
    module: ThingModule
    parameters:
      Scope: { required: false }
`))
	assert.Error(t, err)

	// A cmdlet with no declared parameters is still fine (nothing to constrain).
	_, err = parseAllowlist([]byte(`
version: 1
allowed_cmdlets:
  Get-Thing:
    module: ThingModule
`))
	assert.NoError(t, err)
}

func TestPatternIsAnchored(t *testing.T) {
	// Patterns must match the ENTIRE value, not a substring: an unanchored
	// 'PROD-CL01' would otherwise also accept "PROD-CL01' OR '1'='1".
	al, err := parseAllowlist([]byte(`
version: 1
allowed_cmdlets:
  Get-ClusterNode:
    module: FailoverClusters
    parameters:
      Cluster: { pattern: 'PROD-CL01' }
`))
	require.NoError(t, err)

	assert.NoError(t, al.validateInstance(&instanceConfig{
		Cmdlet:     "Get-ClusterNode",
		Parameters: []parameterEntry{{Name: "Cluster", Value: "PROD-CL01"}},
	}))

	assert.Error(t, al.validateInstance(&instanceConfig{
		Cmdlet:     "Get-ClusterNode",
		Parameters: []parameterEntry{{Name: "Cluster", Value: "PROD-CL01' OR '1'='1"}},
	}))
}

func TestParseAllowlistPatternErrorNamesTheAdminsPattern(t *testing.T) {
	// An unterminated '[' swallows the injected \z, so Go reports an invalid
	// escape sequence for a construct that is not in the allowlist at all.
	_, err := parseAllowlist([]byte(
		"version: 1\nallowed_cmdlets:\n  Get-Service:\n    module: \"*\"\n" +
			"    parameters:\n      Name: { pattern: '[A-Za-z' }\n"))
	require.Error(t, err)

	assert.Contains(t, err.Error(), "missing closing ]")
	assert.Contains(t, err.Error(), `"[A-Za-z"`)
	// The anchoring wrapper must not leak into the message.
	assert.NotContains(t, err.Error(), `\z`)
	assert.NotContains(t, err.Error(), "(?:")
}

func TestParseAllowlistRejectsRE2UnsupportedSyntax(t *testing.T) {
	// RE2 has no lookaround and no backreferences, so a pattern ported from
	// .NET or PCRE fails at load rather than silently behaving differently.
	for _, pattern := range []string{`(?=x)abc`, `(a)\1`} {
		_, err := parseAllowlist([]byte(
			"version: 1\nallowed_cmdlets:\n  Get-Service:\n    module: \"*\"\n" +
				"    parameters:\n      Name: { pattern: '" + pattern + "' }\n"))
		require.Error(t, err, "pattern %q", pattern)
		assert.Contains(t, err.Error(), "invalid pattern")
	}
}

func TestPatternIsCaseSensitive(t *testing.T) {
	// Go regexps are case-sensitive; (?i) is the documented workaround.
	check := func(pattern, value string) error {
		al, err := parseAllowlist([]byte(
			"version: 1\nallowed_cmdlets:\n  Get-Service:\n    module: \"*\"\n" +
				"    parameters:\n      Name: { pattern: '" + pattern + "' }\n"))
		require.NoError(t, err)
		return al.validateInstance(&instanceConfig{
			Cmdlet:     "Get-Service",
			Parameters: []parameterEntry{{Name: "Name", Value: value}},
		})
	}
	assert.NoError(t, check("dnscache", "dnscache"))
	assert.Error(t, check("dnscache", "Dnscache"))
	// (?i) survives the \A(?:...)\z wrapper, since it scopes to the group.
	assert.NoError(t, check("(?i)dnscache", "Dnscache"))
}

func TestParseAllowlistRequiresModule(t *testing.T) {
	// A cmdlet entry without a module is rejected (strict, secure-by-default).
	_, err := parseAllowlist([]byte("version: 1\nallowed_cmdlets:\n  Get-Service:\n    parameters:\n      Name: {}\n"))
	assert.Error(t, err)

	// "*" is the explicit opt-out and is accepted.
	_, err = parseAllowlist([]byte("version: 1\nallowed_cmdlets:\n  Get-Service:\n    module: \"*\"\n"))
	assert.NoError(t, err)
}
