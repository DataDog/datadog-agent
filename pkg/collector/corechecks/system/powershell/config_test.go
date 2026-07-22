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

func TestParseInstanceConfigPositional(t *testing.T) {
	data := []byte(`
cmdlet: Get-ClusterNode
name: failover_cluster_node
filters:
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

	require.Len(t, inst.Filters, 1)
	assert.Equal(t, "Cluster", inst.Filters[0].Name)
	assert.Equal(t, "PROD-CL01", inst.Filters[0].Value)

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
filters:
  - name: Name
    value: Spooler
`)
	inst, err := parseInstanceConfig(data)
	require.NoError(t, err)
	require.Len(t, inst.Metrics, 1)
	assert.Equal(t, "Status", inst.Metrics[0].Property)
	assert.Equal(t, "service.status", inst.Metrics[0].Name)
	require.Len(t, inst.Filters, 1)
	assert.Equal(t, "Name", inst.Filters[0].Name)
	assert.Equal(t, "Spooler", inst.Filters[0].Value)
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
