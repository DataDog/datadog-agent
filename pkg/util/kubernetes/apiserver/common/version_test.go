// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

func TestKubeServerVersion(t *testing.T) {
	tests := []struct {
		name        string
		loadFunc    func()
		fakeVersion *version.Info
		want        *version.Info
		wantErr     bool
		wantFunc    func(t *testing.T, f *fakediscovery.FakeDiscovery)
	}{

		{
			name:        "nominal case",
			loadFunc:    func() { cache.Cache.Flush() },
			fakeVersion: &version.Info{Major: "1", Minor: "17+"},
			want:        &version.Info{Major: "1", Minor: "17+"},
			wantErr:     false,
			wantFunc: func(t *testing.T, f *fakediscovery.FakeDiscovery) {
				_, found := cache.Cache.Get(serverVersionCacheKey)
				assert.True(t, found)
				assert.NotEmpty(t, f.Actions())
			},
		},
		{
			name: "get from cache",
			loadFunc: func() {
				cache.Cache.Flush()
				cache.Cache.Set(serverVersionCacheKey, &version.Info{Major: "1", Minor: "17+"}, time.Hour)
			},
			fakeVersion: &version.Info{Major: "1", Minor: "17+"},
			want:        &version.Info{Major: "1", Minor: "17+"},
			wantErr:     false,
			wantFunc: func(t *testing.T, f *fakediscovery.FakeDiscovery) {
				_, found := cache.Cache.Get(serverVersionCacheKey)
				assert.True(t, found)
				assert.Empty(t, f.Actions())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fakeclientset.NewSimpleClientset()
			fakeDiscovery, ok := client.Discovery().(*fakediscovery.FakeDiscovery)
			assert.True(t, ok)
			fakeDiscovery.FakedServerVersion = tt.fakeVersion

			tt.loadFunc()
			got, err := KubeServerVersion(fakeDiscovery, 2*time.Second)
			assert.Equal(t, tt.wantErr, err != nil)
			assert.Equal(t, tt.want, got)
			tt.wantFunc(t, fakeDiscovery)
		})
	}
}
