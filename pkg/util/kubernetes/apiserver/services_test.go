// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/util/sets"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
)

func TestNamespacesPodsStringsSet(t *testing.T) {
	mapper := apiv1.NewNamespacesPodsStringsSet()

	mapper.Set("default", "pod1", "svc1")
	mapper.Set("default", "pod2", "svc1")
	mapper.Set("default", "pod3", "svc2")

	require.Equal(t, 3, len(mapper["default"]))
	assert.Equal(t, sets.NewString("svc1"), mapper["default"]["pod1"])

	mapper.Delete("default", "svc1")
	require.Equal(t, 1, len(mapper["default"]))
	assert.Equal(t, sets.NewString("svc2"), mapper["default"]["pod3"])

	mapper.Delete("default", "svc2")

	// No more pods in default namespace.
	_, ok := mapper["default"]
	require.False(t, ok, "default namespace still exists")
}
