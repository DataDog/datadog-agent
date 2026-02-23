// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"testing"
	"time"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/version"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

const serverVersionCacheKey = "kubeServerVersion"

func setupFakeAPIClient() {
	globalAPIClientOnce.Do(initAPIClient)

	// Create a minimal fake client for GetAPIClient
	fakeClient := fakeclientset.NewClientset()
	globalAPIClient.Cl = fakeClient

	// Trigger the retry to ensure GetAPIClient() succeeds.
	_ = globalAPIClient.initRetry.SetupRetrier(&retry.Config{
		Name: "test-apiserver",
		AttemptMethod: func() error {
			// Always succeed in tests
			return nil
		},
		Strategy: retry.JustTesting,
	})
	_ = globalAPIClient.initRetry.TriggerRetry()
}

func TestUseEndpointSlices(t *testing.T) {
	tests := []struct {
		name           string
		configEnabled  bool
		version        *version.Info
		expectedResult bool
	}{
		{
			name:           "version below 1.21 should return false",
			configEnabled:  true,
			version:        &version.Info{Major: "1", Minor: "20", GitVersion: "v1.20.5"},
			expectedResult: false,
		},
		{
			name:           "config disabled with version 1.21+ returns false",
			configEnabled:  false,
			version:        &version.Info{Major: "1", Minor: "30", GitVersion: "v1.30.0"},
			expectedResult: false,
		},
		{
			name:           "version exactly 1.21.0 should return true",
			configEnabled:  true,
			version:        &version.Info{Major: "1", Minor: "21", GitVersion: "v1.21.0"},
			expectedResult: true,
		},
		{
			name:           "version 1.21+ should return true",
			configEnabled:  true,
			version:        &version.Info{Major: "1", Minor: "30", GitVersion: "v1.30.0"},
			expectedResult: true,
		},
		{
			name:           "pre-release version 1.21 returns false",
			configEnabled:  true,
			version:        &version.Info{Major: "1", Minor: "21", GitVersion: "v1.21.0-beta"},
			expectedResult: false,
		},
		{
			name:           "pre-release version 1.23 returns true",
			configEnabled:  true,
			version:        &version.Info{Major: "1", Minor: "23", GitVersion: "v1.23.0-beta"},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer cache.Cache.Delete(endpointSlicesCacheKey)

			pkgconfigsetup.Datadog().Set("kubernetes_use_endpoint_slices", tt.configEnabled, pkgconfigmodel.SourceFile)
			cache.Cache.Set(serverVersionCacheKey, tt.version, time.Hour)

			setupFakeAPIClient()

			result := UseEndpointSlices()
			assert.Equal(t, tt.expectedResult, result, tt.name)
			use, found := cache.Cache.Get(endpointSlicesCacheKey)
			assert.True(t, found)
			assert.Equal(t, tt.expectedResult, use.(bool))
		})
	}
}
