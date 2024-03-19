// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package ecs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitClusterID(t *testing.T) {
	id1 := initClusterID(123456789012, "us-east-1", "ecs-cluster-1")
	require.Equal(t, "34616234-6562-3536-3733-656534636532", id1)

	// same account, same region, different cluster name
	id2 := initClusterID(123456789012, "us-east-1", "ecs-cluster-2")
	require.Equal(t, "31643131-3131-3263-3331-383136383336", id2)

	// same account, different region, same cluster name
	id3 := initClusterID(123456789012, "us-east-2", "ecs-cluster-1")
	require.Equal(t, "64663464-6662-3232-3635-646166613230", id3)

	// different account, same region, same cluster name
	id4 := initClusterID(123456789013, "us-east-1", "ecs-cluster-1")
	require.Equal(t, "61623431-6137-6231-3136-366464643761", id4)
}
