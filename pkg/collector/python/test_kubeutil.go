// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test && kubelet

package python

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

import "C"

var testConnections map[string]string

func testGetKubeletConnectionInfoCached(t *testing.T) {
	cache.Cache.Set(kubeletCacheKey, string("CACHED DATA"), 1*time.Minute)
	defer cache.Cache.Delete(kubeletCacheKey)

	var payload *C.char
	GetKubeletConnectionInfo(&payload)

	assert.Equal(t, "CACHED DATA", C.GoString(payload))
}

func getConnectionsMock() map[string]string {
	return testConnections
}

func testGetKubeletConnectionInfoNotCached(t *testing.T) {
	getConnectionsFunc = getConnectionsMock
	defer func() { getConnectionsFunc = getConnections }()

	// making sure the cache is empty
	cache.Cache.Delete(kubeletCacheKey)

	testConnections = map[string]string{
		"conn1": "a",
		"conn2": "b",
	}

	var payload *C.char
	GetKubeletConnectionInfo(&payload)
	assert.Equal(t, "conn1: a\nconn2: b\n", C.GoString(payload))

	testConnections = map[string]string{"conn3": "c"}

	// testing caching
	GetKubeletConnectionInfo(&payload)
	assert.Equal(t, "conn1: a\nconn2: b\n", C.GoString(payload))
}
