// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestStoreGenerators(t *testing.T) {
	// Define tests
	tests := []struct {
		name                    string
		cfg                     map[string]bool
		expectedStoresGenerator []storeGenerator
	}{
		{
			name: "All configurations disabled",
			cfg: map[string]bool{
				"cluster_agent.collect_kubernetes_tags": false,
				"language_detection.enabled":            false,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore},
		},
		{
			name: "Kubernetes tags enabled",
			cfg: map[string]bool{
				"cluster_agent.collect_kubernetes_tags": true,
				"language_detection.enabled":            false,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore, newPodStore},
		},
		{
			name: "Language detection enabled",
			cfg: map[string]bool{
				"cluster_agent.collect_kubernetes_tags": false,
				"language_detection.enabled":            true,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore, newDeploymentStore},
		},
		{
			name: "All configurations enabled",
			cfg: map[string]bool{
				"cluster_agent.collect_kubernetes_tags": true,
				"language_detection.enabled":            true,
			},
			expectedStoresGenerator: []storeGenerator{newNodeStore, newPodStore, newDeploymentStore},
		},
	}

	// Run test for each testcase
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
			for k, v := range tt.cfg {
				cfg.Set(k, v)
			}
			expectedStores := collectResultStoreGenerator(tt.expectedStoresGenerator)
			stores := collectResultStoreGenerator(storeGenerators(cfg))

			assert.Equal(t, expectedStores, stores)
		})
	}
}

func collectResultStoreGenerator(funcs []storeGenerator) []*reflectorStore {
	var stores []*reflectorStore
	for _, f := range funcs {
		_, s := f(nil, nil, nil)
		stores = append(stores, s)
	}
	return stores
}
