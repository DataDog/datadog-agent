// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sharding

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildResourceColocation(t *testing.T) {
	colocation, err := BuildResourceColocation(
		[][]string{
			{"deployments", "replicasets", "deployments"},
			{"pods"},
			{},
		},
		[]string{"deployments", "replicasets", "pods"},
	)

	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"deployments": "deployments",
		"replicasets": "deployments",
		"pods":        "pods",
	}, colocation)
}

func TestBuildResourceColocationRejectsMultipleGroups(t *testing.T) {
	_, err := BuildResourceColocation(
		[][]string{
			{"deployments", "replicasets"},
			{"replicasets", "pods"},
		},
		[]string{"deployments", "replicasets", "pods"},
	)

	require.EqualError(t, err, `resource "replicasets" appears in multiple colocation groups`)
}

func TestBuildResourceColocationRejectsDisabledCollector(t *testing.T) {
	_, err := BuildResourceColocation(
		[][]string{{"deployments", "replicasets"}},
		[]string{"deployments"},
	)

	require.EqualError(t, err, `colocated resource "replicasets" is not an enabled collector`)
}
