// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package powershell

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchemaAcceptsTupleForm(t *testing.T) {
	data := []byte(`
cmdlet: Get-ClusterNode
name: failover_cluster_node
parameters:
  - [Cluster, PROD-CL01]
metrics:
  - [NodeWeight, cluster.node.weight, gauge]
tag_by:
  - Name AS node
tags:
  - "role:db"
tag_queries:
  - [Id, Get-ClusterGroup, OwnerNode, Name AS owner_group]
`)
	assert.NoError(t, validateInstanceSchema(data))
}

func TestSchemaAcceptsMappingForm(t *testing.T) {
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
	assert.NoError(t, validateInstanceSchema(data))
}

func TestSchemaAcceptsMetricMapping(t *testing.T) {
	data := []byte(`
cmdlet: Get-NetAdapter
metrics:
  - property: Status
    name: adapter.status
    mapping:
      Up: 1
      Down: 0
    default_value: -1
`)
	assert.NoError(t, validateInstanceSchema(data))
}

func TestSchemaRejectsNonNumericMappingValue(t *testing.T) {
	data := []byte(`
cmdlet: Get-NetAdapter
metrics:
  - property: Status
    name: adapter.status
    mapping:
      Up: sometimes
    default_value: -1
`)
	assert.Error(t, validateInstanceSchema(data))
}

func TestSchemaRejectsNonNumericDefaultValue(t *testing.T) {
	data := []byte(`
cmdlet: Get-NetAdapter
metrics:
  - property: Status
    name: adapter.status
    default_value: none
`)
	assert.Error(t, validateInstanceSchema(data))
}

func TestSchemaAcceptsWhereForms(t *testing.T) {
	data := []byte(`
cmdlet: Get-SmbShare
metrics:
  - [1, share]
where:
  - [Path, notlike, '*LocalsplOnly*']
  - property: Length
    op: gt
    value: 1024
`)
	assert.NoError(t, validateInstanceSchema(data))
}

func TestSchemaRejectsMalformedWhereTuple(t *testing.T) {
	// A 2-element tuple matches neither the array branch (exactly 3) nor the object.
	data := []byte("cmdlet: Get-SmbShare\nmetrics:\n  - [1, share]\nwhere:\n  - [Path, notlike]\n")
	assert.Error(t, validateInstanceSchema(data))
}

func TestSchemaRejectsUnsupportedWhereOperator(t *testing.T) {
	// The operator enum is duplicated in the schema so a typo is reported alongside
	// any other config error instead of aborting at the first one.
	data := []byte(`
cmdlet: Get-SmbShare
metrics:
  - [1, share]
where:
  - property: Path
    op: contains
    value: x
`)
	assert.Error(t, validateInstanceSchema(data))
}

func TestSchemaAllowsCommonInstanceKeys(t *testing.T) {
	// Keys the Agent injects/accepts must not be rejected.
	data := []byte(`
cmdlet: Get-Service
min_collection_interval: 30
service: my-service
empty_default_hostname: true
metrics:
  - [Status, status, gauge]
`)
	assert.NoError(t, validateInstanceSchema(data))
}

func TestSchemaNegativeTimeoutIsValid(t *testing.T) {
	// A negative timeout passes schema (it is an integer); it is coerced to the
	// default later in parseInstanceConfig.
	data := []byte("cmdlet: Get-Service\ntimeout: -5\nmetrics:\n  - [Status, status, gauge]\n")
	assert.NoError(t, validateInstanceSchema(data))
}

func TestSchemaRejectsMissingCmdlet(t *testing.T) {
	data := []byte("metrics:\n  - [Status, status, gauge]\n")
	assert.Error(t, validateInstanceSchema(data))
}

func TestSchemaRejectsMissingMetrics(t *testing.T) {
	data := []byte("cmdlet: Get-Service\n")
	assert.Error(t, validateInstanceSchema(data))
}

func TestSchemaRejectsEmptyMetrics(t *testing.T) {
	data := []byte("cmdlet: Get-Service\nmetrics: []\n")
	assert.Error(t, validateInstanceSchema(data))
}

func TestSchemaRejectsInvalidMetricTypeMappingForm(t *testing.T) {
	data := []byte(`
cmdlet: Get-Service
metrics:
  - property: Status
    name: service.status
    type: counter
`)
	assert.Error(t, validateInstanceSchema(data))
}

func TestSchemaRejectsMalformedMetricTuple(t *testing.T) {
	// A one-element tuple matches neither the array (min 2) nor the object branch.
	data := []byte("cmdlet: Get-Service\nmetrics:\n  - [Status]\n")
	assert.Error(t, validateInstanceSchema(data))
}

func TestSchemaRejectsNonIntegerTimeout(t *testing.T) {
	data := []byte("cmdlet: Get-Service\ntimeout: soon\nmetrics:\n  - [Status, status, gauge]\n")
	assert.Error(t, validateInstanceSchema(data))
}

func TestSchemaRejectsNonStringCmdlet(t *testing.T) {
	data := []byte("cmdlet: 5\nmetrics:\n  - [Status, status, gauge]\n")
	assert.Error(t, validateInstanceSchema(data))
}
