// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package instrumentation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMapCELRules(t *testing.T) {
	rules := BuildMapCELRules("container.pod.annotations", map[string]string{
		"b-key": "val-b",
		"a-key": "val-a",
	})

	require.Len(t, rules, 2)
	assert.Equal(t, `container.pod.annotations["a-key"] == "val-a"`, rules[0])
	assert.Equal(t, `container.pod.annotations["b-key"] == "val-b"`, rules[1])
}

func TestBuildMapCELRules_Labels(t *testing.T) {
	rules := BuildMapCELRules("container.pod.labels", map[string]string{"app": "nginx"})
	require.Len(t, rules, 1)
	assert.Equal(t, `container.pod.labels["app"] == "nginx"`, rules[0])
}

func TestBuildMapCELRules_Empty(t *testing.T) {
	assert.Nil(t, BuildMapCELRules("container.pod.labels", nil))
}
