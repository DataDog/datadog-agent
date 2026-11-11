// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package configimpl

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKubeClusterUIDResolver(t *testing.T) {
	const uid = "d3b07384-d9a7-4e1c-8b6f-6e2a1c7f0a11"
	var calls atomic.Int32
	release := make(chan struct{})
	r := &kubeClusterUIDResolver{
		getClusterID: func() (string, error) {
			<-release
			if calls.Add(1) < 3 {
				return "", errors.New("cluster agent unavailable")
			}
			return uid, nil
		},
		minRetry: time.Millisecond,
		maxRetry: 2 * time.Millisecond,
	}

	// get does not wait for the resolution
	assert.Equal(t, "", r.get())
	assert.Equal(t, "", r.get())
	close(release)

	require.Eventually(t, func() bool { return r.get() == uid }, 5*time.Second, time.Millisecond)
	assert.Equal(t, int32(3), calls.Load(), "retries until the cluster UID is resolved")
	assert.Equal(t, uid, r.get())
	assert.Equal(t, int32(3), calls.Load(), "the cluster UID is cached")
}
