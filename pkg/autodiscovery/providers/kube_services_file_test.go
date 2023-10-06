// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"

	"github.com/stretchr/testify/assert"
)

func TestToKubernetesServiceChecks(t *testing.T) {
	tests := []struct {
		name    string
		configs []integration.Config
		want    []integration.Config
	}{
		{
			name: "nominal case",
			configs: []integration.Config{
				{
					Name:                  "check",
					Instances:             []integration.Data{integration.Data("foo: bar")},
					AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{KubeService: kubeNsName("svc-ns", "svc-name")}},
				},
			},
			want: []integration.Config{
				{
					Name:                  "check",
					Instances:             []integration.Data{integration.Data("foo: bar")},
					ADIdentifiers:         []string{"kube_service://svc-ns/svc-name"},
					Provider:              names.KubeServicesFile,
					AdvancedADIdentifiers: nil,
					ClusterCheck:          true,
				},
			},
		},
		{
			name: "no advanced AD",
			configs: []integration.Config{
				{
					Name:      "check",
					Instances: []integration.Data{integration.Data("foo: bar")},
				},
			},
			want: []integration.Config{},
		},
		{
			name: "ignore endpoints advanced AD (1)",
			configs: []integration.Config{
				{
					Name:                  "check",
					Instances:             []integration.Data{integration.Data("foo: bar")},
					AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{KubeEndpoints: kubeNsName("svc-ns", "svc-name")}},
				},
			},
			want: []integration.Config{},
		},
		{
			name: "ignore endpoints advanced AD (2)",
			configs: []integration.Config{
				{
					Name:      "check",
					Instances: []integration.Data{integration.Data("foo: bar")},
					AdvancedADIdentifiers: []integration.AdvancedADIdentifier{{
						KubeService:   kubeNsName("svc-ns", "svc-name"),
						KubeEndpoints: kubeNsName("svc-ns", "svc-name"),
					}},
				},
			},
			want: []integration.Config{
				{
					Name:                  "check",
					Instances:             []integration.Data{integration.Data("foo: bar")},
					ADIdentifiers:         []string{"kube_service://svc-ns/svc-name"},
					Provider:              names.KubeServicesFile,
					AdvancedADIdentifiers: nil,
					ClusterCheck:          true,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualValues(t, tt.want, toKubernetesServiceChecks(tt.configs))
		})
	}
}

func kubeNsName(ns, name string) integration.KubeNamespacedName {
	return integration.KubeNamespacedName{Name: name, Namespace: ns}
}
