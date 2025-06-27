// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proto

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestProtobufConfigFromAutodiscoveryConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    *integration.Config
		expected *core.Config
	}{
		{
			name: "all fields set",
			input: &integration.Config{
				Name: "test_config",
				Instances: []integration.Data{
					[]byte("instance1"),
					[]byte("instance2"),
				},
				InitConfig:    []byte("init_config"),
				MetricConfig:  []byte("metric_config"),
				LogsConfig:    []byte("logs_config"),
				ADIdentifiers: []string{"ad_identifier1", "ad_identifier2"},
				AdvancedADIdentifiers: []integration.AdvancedADIdentifier{
					{
						KubeService: integration.KubeNamespacedName{
							Name:      "service1",
							Namespace: "namespace1",
						},
						KubeEndpoints: integration.KubeEndpointsIdentifier{
							KubeNamespacedName: integration.KubeNamespacedName{
								Name:      "endpoint1",
								Namespace: "namespace1",
							},
							Resolve: "auto",
						},
					},
				},
				Provider:                "provider",
				ServiceID:               "service_id",
				TaggerEntity:            "tagger_entity",
				ClusterCheck:            true,
				NodeName:                "node_name",
				Source:                  "source",
				IgnoreAutodiscoveryTags: true,
				MetricsExcluded:         true,
				LogsExcluded:            true,
			},
			expected: &core.Config{
				Name: "test_config",
				Instances: [][]byte{
					[]byte("instance1"),
					[]byte("instance2"),
				},
				InitConfig:    []byte("init_config"),
				MetricConfig:  []byte("metric_config"),
				LogsConfig:    []byte("logs_config"),
				AdIdentifiers: []string{"ad_identifier1", "ad_identifier2"},
				AdvancedAdIdentifiers: []*core.AdvancedADIdentifier{
					{
						KubeService: &core.KubeNamespacedName{
							Name:      "service1",
							Namespace: "namespace1",
						},
						KubeEndpoints: &core.KubeEndpointsIdentifier{
							KubeNamespacedName: &core.KubeNamespacedName{
								Name:      "endpoint1",
								Namespace: "namespace1",
							},
							Resolve: "auto",
						},
					},
				},
				Provider:                "provider",
				ServiceId:               "service_id",
				TaggerEntity:            "tagger_entity",
				ClusterCheck:            true,
				NodeName:                "node_name",
				Source:                  "source",
				IgnoreAutodiscoveryTags: true,
				MetricsExcluded:         true,
				LogsExcluded:            true,
			},
		},
		{
			name: "some fields set",
			input: &integration.Config{
				Name: "test_config",
				Instances: []integration.Data{
					[]byte("instance1"),
				},
				InitConfig:              []byte("init_config"),
				MetricConfig:            []byte("metric_config"),
				LogsConfig:              []byte("logs_config"),
				ADIdentifiers:           []string{"ad_identifier1"},
				Provider:                "provider",
				ServiceID:               "service_id",
				TaggerEntity:            "tagger_entity",
				ClusterCheck:            true,
				NodeName:                "node_name",
				Source:                  "source",
				IgnoreAutodiscoveryTags: true,
				MetricsExcluded:         true,
				LogsExcluded:            true,
			},
			expected: &core.Config{
				Name: "test_config",
				Instances: [][]byte{
					[]byte("instance1"),
				},
				InitConfig:              []byte("init_config"),
				MetricConfig:            []byte("metric_config"),
				LogsConfig:              []byte("logs_config"),
				AdIdentifiers:           []string{"ad_identifier1"},
				Provider:                "provider",
				ServiceId:               "service_id",
				TaggerEntity:            "tagger_entity",
				ClusterCheck:            true,
				NodeName:                "node_name",
				Source:                  "source",
				IgnoreAutodiscoveryTags: true,
				MetricsExcluded:         true,
				LogsExcluded:            true,
				AdvancedAdIdentifiers:   []*core.AdvancedADIdentifier{},
			},
		},
		{
			name:  "no fields set",
			input: &integration.Config{},
			expected: &core.Config{
				Instances:             [][]byte{},
				AdvancedAdIdentifiers: []*core.AdvancedADIdentifier{},
			},
		},
		{
			"nil",
			nil,
			nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ProtobufConfigFromAutodiscoveryConfig(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestAutodiscoveryConfigFromProtobufConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    *core.Config
		expected integration.Config
	}{
		{
			name: "all fields set",
			input: &core.Config{
				Name: "test_config",
				Instances: [][]byte{
					[]byte("instance1"),
					[]byte("instance2"),
				},
				InitConfig:    []byte("init_config"),
				MetricConfig:  []byte("metric_config"),
				LogsConfig:    []byte("logs_config"),
				AdIdentifiers: []string{"ad_identifier1", "ad_identifier2"},
				AdvancedAdIdentifiers: []*core.AdvancedADIdentifier{
					{
						KubeService: &core.KubeNamespacedName{
							Name:      "service1",
							Namespace: "namespace1",
						},
						KubeEndpoints: &core.KubeEndpointsIdentifier{
							KubeNamespacedName: &core.KubeNamespacedName{
								Name:      "endpoint1",
								Namespace: "namespace1",
							},
							Resolve: "auto",
						},
					},
				},
				Provider:                "provider",
				ServiceId:               "service_id",
				TaggerEntity:            "tagger_entity",
				ClusterCheck:            true,
				NodeName:                "node_name",
				Source:                  "source",
				IgnoreAutodiscoveryTags: true,
				MetricsExcluded:         true,
				LogsExcluded:            true,
			},
			expected: integration.Config{
				Name: "test_config",
				Instances: []integration.Data{
					[]byte("instance1"),
					[]byte("instance2"),
				},
				InitConfig:    []byte("init_config"),
				MetricConfig:  []byte("metric_config"),
				LogsConfig:    []byte("logs_config"),
				ADIdentifiers: []string{"ad_identifier1", "ad_identifier2"},
				AdvancedADIdentifiers: []integration.AdvancedADIdentifier{
					{
						KubeService: integration.KubeNamespacedName{
							Name:      "service1",
							Namespace: "namespace1",
						},
						KubeEndpoints: integration.KubeEndpointsIdentifier{
							KubeNamespacedName: integration.KubeNamespacedName{
								Name:      "endpoint1",
								Namespace: "namespace1",
							},
							Resolve: "auto",
						},
					},
				},
				Provider:                "provider",
				ServiceID:               "service_id",
				TaggerEntity:            "tagger_entity",
				ClusterCheck:            true,
				NodeName:                "node_name",
				Source:                  "source",
				IgnoreAutodiscoveryTags: true,
				MetricsExcluded:         true,
				LogsExcluded:            true,
			},
		},
		{
			name: "some fields set",
			input: &core.Config{
				Name: "test_config",
				Instances: [][]byte{
					[]byte("instance1"),
				},
				InitConfig:              []byte("init_config"),
				MetricConfig:            []byte("metric_config"),
				LogsConfig:              []byte("logs_config"),
				AdIdentifiers:           []string{"ad_identifier1"},
				Provider:                "provider",
				ServiceId:               "service_id",
				TaggerEntity:            "tagger_entity",
				ClusterCheck:            true,
				NodeName:                "node_name",
				Source:                  "source",
				IgnoreAutodiscoveryTags: true,
				MetricsExcluded:         true,
				LogsExcluded:            true,
			},
			expected: integration.Config{
				Name: "test_config",
				Instances: []integration.Data{
					[]byte("instance1"),
				},
				InitConfig:              []byte("init_config"),
				MetricConfig:            []byte("metric_config"),
				LogsConfig:              []byte("logs_config"),
				ADIdentifiers:           []string{"ad_identifier1"},
				Provider:                "provider",
				ServiceID:               "service_id",
				TaggerEntity:            "tagger_entity",
				ClusterCheck:            true,
				NodeName:                "node_name",
				Source:                  "source",
				IgnoreAutodiscoveryTags: true,
				MetricsExcluded:         true,
				LogsExcluded:            true,
				AdvancedADIdentifiers:   []integration.AdvancedADIdentifier{},
			},
		},
		{
			name:  "no fields set",
			input: &core.Config{},
			expected: integration.Config{
				AdvancedADIdentifiers: []integration.AdvancedADIdentifier{},
				Instances:             []integration.Data{},
			},
		},
		{
			"nil",
			nil,
			integration.Config{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := AutodiscoveryConfigFromProtobufConfig(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}
