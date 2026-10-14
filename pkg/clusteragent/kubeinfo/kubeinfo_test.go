// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeinfo

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestCheckServerVersion(t *testing.T) {
	tests := []struct {
		name            string
		serverVersion   *version.Info
		err             error
		wantVersion     string
		wantUnsupported bool
	}{
		{name: "supported", serverVersion: &version.Info{GitVersion: "v1.30.2"}, wantVersion: "v1.30.2"},
		{name: "unsupported", serverVersion: &version.Info{GitVersion: "v1.22.17"}, wantVersion: "v1.22.17", wantUnsupported: true},
		{name: "discovery error", err: errors.New("forbidden")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset(t)
			// KubeServerVersion caches the version globally.
			cache.Cache.Flush()
			t.Cleanup(cache.Cache.Flush)
			previousTimeout := serverVersionTimeout
			serverVersionTimeout = 100 * time.Millisecond
			t.Cleanup(func() { serverVersionTimeout = previousTimeout })

			discoveryCl := fakeclientset.NewClientset().Discovery().(*fakediscovery.FakeDiscovery)
			discoveryCl.FakedServerVersion = tt.serverVersion
			if tt.err != nil {
				discoveryCl.PrependReactor("get", "version", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.err
				})
			}

			CheckServerVersion(discoveryCl)

			i := getInfo()
			assert.Equal(t, tt.wantVersion, i.serverVersion)
			assert.Equal(t, tt.wantUnsupported, i.unsupported)
		})
	}
}

func TestSetClusterInfoKeepsServerVersion(t *testing.T) {
	reset(t)
	setServerVersion(&version.Info{GitVersion: "v1.22.17"})
	SetClusterInfo("my-cluster", "id", "gke")

	i := getInfo()
	assert.Equal(t, "my-cluster", i.clusterName)
	assert.Equal(t, "id", i.clusterID)
	assert.Equal(t, "gke", i.distribution)
	assert.Equal(t, "v1.22.17", i.serverVersion)
	assert.True(t, i.unsupported)
}
