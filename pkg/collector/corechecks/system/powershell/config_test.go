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

func TestParseInstanceConfigPositional(t *testing.T) {
	data := []byte(`
cmdlet: Get-ClusterNode
name: failover_cluster_node
parameters:
  - [Cluster, PROD-CL01]
metrics:
  - [NodeWeight, cluster.node.weight, gauge]
tag_by:
  - Name AS node
  - State
tags:
  - "role:db"
tag_queries:
  - [Id, Get-ClusterGroup, OwnerNode, Name AS owner_group]
`)
	inst, err := parseInstanceConfig(data)
	require.NoError(t, err)

	assert.Equal(t, "Get-ClusterNode", inst.Cmdlet)
	assert.Equal(t, "failover_cluster_node", inst.Name)

	require.Len(t, inst.Parameters, 1)
	assert.Equal(t, "Cluster", inst.Parameters[0].Name)
	assert.Equal(t, "PROD-CL01", inst.Parameters[0].Value)

	require.Len(t, inst.Metrics, 1)
	assert.Equal(t, "NodeWeight", inst.Metrics[0].Property)
	assert.Equal(t, "cluster.node.weight", inst.Metrics[0].Name)
	assert.Equal(t, "gauge", inst.Metrics[0].Type)

	require.Len(t, inst.TagBy, 2)
	assert.Equal(t, "Name", inst.TagBy[0].Property)
	assert.Equal(t, "node", inst.TagBy[0].Alias)
	assert.Equal(t, "State", inst.TagBy[1].Property)
	assert.Equal(t, "state", inst.TagBy[1].Alias) // defaults to lowercased property

	require.Len(t, inst.TagQueries, 1)
	q := inst.TagQueries[0]
	assert.Equal(t, "Id", q.LinkSourceProperty)
	assert.Equal(t, "Get-ClusterGroup", q.TargetCmdlet)
	assert.Equal(t, "OwnerNode", q.LinkTargetProperty)
	assert.Equal(t, "Name", q.TargetProperty)
	assert.Equal(t, "owner_group", q.Alias)

	assert.Equal(t, defaultTimeout, inst.Timeout)
}

func TestParseInstanceConfigMappingForm(t *testing.T) {
	data := []byte(`
cmdlet: Get-Service
metrics:
  - property: Status
    name: service.status
    type: gauge
parameters:
  - name: Name
    value: Spooler
`)
	inst, err := parseInstanceConfig(data)
	require.NoError(t, err)
	require.Len(t, inst.Metrics, 1)
	assert.Equal(t, "Status", inst.Metrics[0].Property)
	assert.Equal(t, "service.status", inst.Metrics[0].Name)
	require.Len(t, inst.Parameters, 1)
	assert.Equal(t, "Name", inst.Parameters[0].Name)
	assert.Equal(t, "Spooler", inst.Parameters[0].Value)
}

func TestMetricTypeDefaultsToGauge(t *testing.T) {
	data := []byte(`
cmdlet: Get-Service
metrics:
  - [Status, service.status]
`)
	inst, err := parseInstanceConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "gauge", inst.Metrics[0].Type)
}

func TestMetricNameOptionalPrefix(t *testing.T) {
	withName := &instanceConfig{Name: "foo"}
	assert.Equal(t, "foo.bar", withName.metricName(&metricEntry{Name: "bar"}))

	noName := &instanceConfig{}
	assert.Equal(t, "bar", noName.metricName(&metricEntry{Name: "bar"}))
}

func TestVirtualMetric(t *testing.T) {
	data := []byte(`
cmdlet: Get-Certificate
metrics:
  - [1, certificates.certificate, gauge]
tag_by:
  - SerialNumber AS sn
`)
	inst, err := parseInstanceConfig(data)
	require.NoError(t, err)
	assert.True(t, inst.Metrics[0].isVirtual())
}

func TestMetricMappingResolution(t *testing.T) {
	data := []byte(`
cmdlet: Get-NetAdapter
metrics:
  - property: Status
    name: adapter.status
    mapping:
      Up: 1
      Down: 0
      Disabled: 0
    default_value: -1
`)
	inst, err := parseInstanceConfig(data)
	require.NoError(t, err)
	m := &inst.Metrics[0]

	// Lookups are case-insensitive: finalize lower-cases the configured keys and
	// resolveValue lower-cases the observed value.
	v, ok := m.resolveValue("Up")
	require.True(t, ok)
	assert.Equal(t, float64(1), v)

	v, ok = m.resolveValue("up")
	require.True(t, ok)
	assert.Equal(t, float64(1), v)

	v, ok = m.resolveValue("DOWN")
	require.True(t, ok)
	assert.Equal(t, float64(0), v)

	// An unmapped value falls back to default_value rather than failing the run —
	// this is what makes vocabulary drift survivable.
	v, ok = m.resolveValue("Degraded")
	require.True(t, ok)
	assert.Equal(t, float64(-1), v)

	// A null property (Select-Object emits one for an absent property) likewise.
	v, ok = m.resolveValue(nil)
	require.True(t, ok)
	assert.Equal(t, float64(-1), v)

	// An already-numeric value bypasses the mapping entirely: toFloat runs first.
	v, ok = m.resolveValue(float64(7))
	require.True(t, ok)
	assert.Equal(t, float64(7), v)
}

func TestMetricMappingRequiresDefaultValue(t *testing.T) {
	// Without a default, one unexpected enum member would abort the whole
	// collection interval, so the pairing is enforced at parse time.
	_, err := parseInstanceConfig([]byte(
		"cmdlet: Get-X\nmetrics:\n  - property: S\n    name: s\n    mapping: {up: 1}\n"))
	assert.Error(t, err)
}

func TestMetricDefaultValueWithoutMappingIsAllowed(t *testing.T) {
	inst, err := parseInstanceConfig([]byte(
		"cmdlet: Get-X\nmetrics:\n  - property: S\n    name: s\n    default_value: -1\n"))
	require.NoError(t, err)
	v, ok := inst.Metrics[0].resolveValue("not-a-number")
	require.True(t, ok)
	assert.Equal(t, float64(-1), v)
}

func TestMetricDefaultValueZeroIsDistinctFromUnset(t *testing.T) {
	// The reason DefaultValue is an option.Option rather than a bare float64.
	inst, err := parseInstanceConfig([]byte(
		"cmdlet: Get-X\nmetrics:\n  - property: S\n    name: s\n    mapping: {up: 1}\n    default_value: 0\n"))
	require.NoError(t, err)
	v, ok := inst.Metrics[0].resolveValue("unknown")
	require.True(t, ok)
	assert.Equal(t, float64(0), v)
}

func TestMetricResolveValueFailsWithoutMappingOrDefault(t *testing.T) {
	// A metric configuring neither keeps the existing loud behavior, so a typo'd
	// property still surfaces in `agent status` instead of silently emitting nothing.
	inst, err := parseInstanceConfig([]byte("cmdlet: Get-X\nmetrics:\n  - [Status, s]\n"))
	require.NoError(t, err)
	_, ok := inst.Metrics[0].resolveValue("Running")
	assert.False(t, ok)
}

func TestMetricMappingRejectedOnVirtualMetric(t *testing.T) {
	// A virtual metric always submits 1, so there is no value to translate.
	_, err := parseInstanceConfig([]byte(
		"cmdlet: Get-X\nmetrics:\n  - property: 1\n    name: s\n    mapping: {up: 1}\n    default_value: 0\n"))
	assert.Error(t, err)

	_, err = parseInstanceConfig([]byte(
		"cmdlet: Get-X\nmetrics:\n  - property: 1\n    name: s\n    default_value: 0\n"))
	assert.Error(t, err)
}

func TestMetricMappingRejectsCaseCollidingKeys(t *testing.T) {
	// Lower-casing would silently discard one of these.
	_, err := parseInstanceConfig([]byte(
		"cmdlet: Get-X\nmetrics:\n  - property: S\n    name: s\n    mapping: {Up: 1, up: 0}\n    default_value: -1\n"))
	require.Error(t, err)
	// Named in sorted order, since Go randomizes map iteration.
	assert.Contains(t, err.Error(), `"Up" and "up"`)
}

func TestParseInstanceConfigRejectsMalformedIdentifiers(t *testing.T) {
	// These are all projected with Select-Object or splatted as parameter names,
	// so a malformed one is rejected here rather than failing every run.
	cases := []struct {
		name string
		yaml string
	}{
		{"metric property", "cmdlet: Get-Service\nmetrics:\n  - [Bad Prop, s]\n"},
		{"parameter name", "cmdlet: Get-Service\nmetrics:\n  - [Status, s]\nparameters:\n  - [Bad Name, x]\n"},
		{"tag_by property", "cmdlet: Get-Service\nmetrics:\n  - [Status, s]\ntag_by:\n  - Bad;Prop\n"},
		{"tag_queries link source", "cmdlet: Get-Service\nmetrics:\n  - [Status, s]\ntag_queries:\n  - [Bad;Id, Get-X, Y, Z]\n"},
		{"tag_queries target property", "cmdlet: Get-Service\nmetrics:\n  - [Status, s]\ntag_queries:\n  - [Id, Get-X, Y, Bad;Z]\n"},
		{"tag_queries target cmdlet", "cmdlet: Get-Service\nmetrics:\n  - [Status, s]\ntag_queries:\n  - [Id, Remove-Item, Y, Z]\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseInstanceConfig([]byte(tc.yaml))
			assert.Error(t, err)
		})
	}
}

func TestParseInstanceConfigAllowsDottedMetricNamesAndAliases(t *testing.T) {
	// Only properties and parameter names are identifiers. A metric name and a
	// tag alias are neither, and must not be caught by the checks above.
	inst, err := parseInstanceConfig([]byte(
		"cmdlet: Get-Service\nmetrics:\n  - [Status, my.svc.status]\ntag_by:\n  - Name AS service.name\n"))
	require.NoError(t, err)
	assert.Equal(t, "my.svc.status", inst.Metrics[0].Name)
	assert.Equal(t, "service.name", inst.TagBy[0].Alias)
}

func TestParseInstanceConfigRejectsDuplicateParameters(t *testing.T) {
	// The splat table is keyed case-insensitively, so a duplicate would silently
	// overwrite instead of failing.
	_, err := parseInstanceConfig([]byte(
		"cmdlet: Get-Service\nmetrics:\n  - [Status, s]\nparameters:\n  - [Name, a]\n  - [Name, b]\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than once")

	_, err = parseInstanceConfig([]byte(
		"cmdlet: Get-Service\nmetrics:\n  - [Status, s]\nparameters:\n  - [Name, a]\n  - [name, b]\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than once")
}

func TestParseInstanceConfigMaxOutputBytes(t *testing.T) {
	base := "cmdlet: Get-Service\nmetrics:\n  - [Status, s]\n"

	inst, err := parseInstanceConfig([]byte(base + "max_output_bytes: 2048\n"))
	require.NoError(t, err)
	assert.Equal(t, 2048, inst.MaxOutputBytes)

	// Unset, zero, and negative all fall back to the default.
	for _, cfg := range []string{"", "max_output_bytes: 0\n", "max_output_bytes: -5\n"} {
		inst, err = parseInstanceConfig([]byte(base + cfg))
		require.NoError(t, err, "config %q", cfg)
		assert.Equal(t, defaultMaxOutputBytes, inst.MaxOutputBytes, "config %q", cfg)
	}

	// The ceiling is absolute: it is what bounds Agent memory.
	inst, err = parseInstanceConfig([]byte(base + "max_output_bytes: 999999999\n"))
	require.NoError(t, err)
	assert.Equal(t, maxOutputBytesCeiling, inst.MaxOutputBytes)
}

func TestSelectPropertiesDedup(t *testing.T) {
	inst := &instanceConfig{
		Metrics: []metricEntry{
			{Property: "1", Name: "virtual"}, // virtual, excluded
			{Property: "NodeWeight", Name: "w"},
		},
		TagBy:      []tagByEntry{{Property: "Name", Alias: "node"}, {Property: "NodeWeight", Alias: "nw"}},
		TagQueries: []tagQueryEntry{{LinkSourceProperty: "Id", TargetCmdlet: "Get-X", LinkTargetProperty: "Y", TargetProperty: "Z"}},
	}
	props := inst.selectProperties()
	assert.ElementsMatch(t, []string{"NodeWeight", "Name", "Id"}, props)
}

func TestParseInstanceConfigRejectsNonGetCmdlet(t *testing.T) {
	_, err := parseInstanceConfig([]byte("cmdlet: Remove-Item\nmetrics:\n  - [X, x]\n"))
	assert.Error(t, err)
}

func TestParseInstanceConfigAcceptsScalarParameterValues(t *testing.T) {
	// string, number, and boolean scalars are all accepted.
	inst, err := parseInstanceConfig([]byte(
		"cmdlet: Get-Service\nmetrics:\n  - [Status, s]\nparameters:\n  - [Name, Spooler]\n  - [Depth, 3]\n  - [Recurse, true]\n"))
	require.NoError(t, err)
	require.Len(t, inst.Parameters, 3)
}

func TestParseInstanceConfigRejectsNonScalarParameterValue(t *testing.T) {
	// a list value would be validated as one string but executed as another, so
	// it is rejected at parse time.
	_, err := parseInstanceConfig([]byte(
		"cmdlet: Get-Service\nmetrics:\n  - [Status, s]\nparameters:\n  - name: Name\n    value: [a, b]\n"))
	assert.Error(t, err)

	// a mapping value is likewise rejected.
	_, err = parseInstanceConfig([]byte(
		"cmdlet: Get-Service\nmetrics:\n  - [Status, s]\nparameters:\n  - name: Name\n    value: {k: v}\n"))
	assert.Error(t, err)
}

func TestParseInstanceConfigRequiresCmdletAndMetrics(t *testing.T) {
	_, err := parseInstanceConfig([]byte("metrics:\n  - [X, x]\n"))
	assert.Error(t, err)

	_, err = parseInstanceConfig([]byte("cmdlet: Get-Service\n"))
	assert.Error(t, err)
}

func TestParseInstanceConfigTimeout(t *testing.T) {
	base := "cmdlet: Get-Service\nmetrics:\n  - [Status, s]\n"

	// an explicit positive value is honored
	inst, err := parseInstanceConfig([]byte(base + "timeout: 45\n"))
	require.NoError(t, err)
	assert.Equal(t, 45, inst.Timeout)

	// a non-positive value is invalid: fall back to the default (with a warning)
	inst, err = parseInstanceConfig([]byte(base + "timeout: -5\n"))
	require.NoError(t, err)
	assert.Equal(t, defaultTimeout, inst.Timeout)

	// omitted defaults
	inst, err = parseInstanceConfig([]byte(base))
	require.NoError(t, err)
	assert.Equal(t, defaultTimeout, inst.Timeout)
}

func TestSplitAS(t *testing.T) {
	p, a := splitAS("Name AS node")
	assert.Equal(t, "Name", p)
	assert.Equal(t, "node", a)

	p, a = splitAS("State")
	assert.Equal(t, "State", p)
	assert.Equal(t, "state", a)

	// case-insensitive AS keyword
	p, a = splitAS("Name as node")
	assert.Equal(t, "Name", p)
	assert.Equal(t, "node", a)
}
