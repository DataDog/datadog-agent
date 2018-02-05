// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython,kubelet

// NOTICE: See TestMain function in `utils_test.go` for Python initialization
package py

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func TestGetKubeletConnectionInfoNotFound(t *testing.T) {
	config.Datadog.Set("kubernetes_http_kubelet_port", 0)
	config.Datadog.Set("kubernetes_https_kubelet_port", 0)
	kubelet.ResetGlobalKubeUtil()
	cache.Cache.Delete(kubeletCacheKey)

	check, _ := getCheckInstance("testkubeutil", "TestCheck")
	err := check.Run()
	assert.Nil(t, err)

	warnings := check.GetWarnings()
	require.Len(t, warnings, 1)
	assert.Equal(t, "Kubelet not found", warnings[0].Error())
}

func TestGetKubeletConnectionInfoFromCache(t *testing.T) {
	dummyCreds := map[string]string{
		"url": "https://10.0.0.1:10250",
	}
	cache.Cache.Set(kubeletCacheKey, dummyCreds, 5*time.Minute)

	check, _ := getCheckInstance("testkubeutil", "TestCheck")
	err := check.Run()
	assert.Nil(t, err)

	warnings := check.GetWarnings()
	require.Len(t, warnings, 1)
	assert.Equal(t, "Found kubelet at https://10.0.0.1:10250", warnings[0].Error())
}
