// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package proto provides autodiscovery proto util functions
package proto

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// ProtobufConfigFromAutodiscoveryConfig converts an autodiscovery config to a protobuf config
func ProtobufConfigFromAutodiscoveryConfig(config *integration.Config) *core.Config {
	if config == nil {
		return nil
	}

	instances := [][]byte{}

	for _, instance := range config.Instances {
		instances = append(instances, []byte(instance))
	}

	advancedAdIdentifiers := make([]*core.AdvancedADIdentifier, 0, len(config.AdvancedADIdentifiers))
	for _, advancedAdIdentifier := range config.AdvancedADIdentifiers {
		advancedAdIdentifiers = append(advancedAdIdentifiers, &core.AdvancedADIdentifier{
			KubeService: &core.KubeNamespacedName{
				Name:      advancedAdIdentifier.KubeService.Name,
				Namespace: advancedAdIdentifier.KubeService.Namespace,
			},
			KubeEndpoints: &core.KubeEndpointsIdentifier{
				KubeNamespacedName: &core.KubeNamespacedName{
					Name:      advancedAdIdentifier.KubeEndpoints.Name,
					Namespace: advancedAdIdentifier.KubeEndpoints.Namespace,
				},
				Resolve: advancedAdIdentifier.KubeEndpoints.Resolve,
			},
		})
	}

	return &core.Config{
		Name:                    config.Name,
		Instances:               instances,
		InitConfig:              config.InitConfig,
		MetricConfig:            config.MetricConfig,
		LogsConfig:              config.LogsConfig,
		AdIdentifiers:           config.ADIdentifiers,
		AdvancedAdIdentifiers:   advancedAdIdentifiers,
		Provider:                config.Provider,
		ServiceId:               config.ServiceID,
		TaggerEntity:            config.TaggerEntity,
		ClusterCheck:            config.ClusterCheck,
		NodeName:                config.NodeName,
		Source:                  config.Source,
		IgnoreAutodiscoveryTags: config.IgnoreAutodiscoveryTags,
		MetricsExcluded:         config.MetricsExcluded,
		LogsExcluded:            config.LogsExcluded,
	}
}

// AutodiscoveryConfigFromProtobufConfig converts a protobuf config to an autodiscovery config
func AutodiscoveryConfigFromProtobufConfig(config *core.Config) integration.Config {
	if config == nil {
		return integration.Config{}
	}

	instances := []integration.Data{}

	for _, instance := range config.Instances {
		instances = append(instances, integration.Data(instance))
	}

	advancedAdIdentifiers := make([]integration.AdvancedADIdentifier, 0, len(config.AdvancedAdIdentifiers))
	for _, advancedAdIdentifier := range config.AdvancedAdIdentifiers {
		advancedAdIdentifiers = append(advancedAdIdentifiers, integration.AdvancedADIdentifier{
			KubeService: integration.KubeNamespacedName{
				Name:      advancedAdIdentifier.KubeService.Name,
				Namespace: advancedAdIdentifier.KubeService.Namespace,
			},
			KubeEndpoints: integration.KubeEndpointsIdentifier{
				KubeNamespacedName: integration.KubeNamespacedName{
					Name:      advancedAdIdentifier.KubeEndpoints.KubeNamespacedName.Name,
					Namespace: advancedAdIdentifier.KubeEndpoints.KubeNamespacedName.Namespace,
				},
				Resolve: advancedAdIdentifier.KubeEndpoints.Resolve,
			},
		})
	}

	return integration.Config{
		Name:                    config.Name,
		Instances:               instances,
		InitConfig:              config.InitConfig,
		MetricConfig:            config.MetricConfig,
		LogsConfig:              config.LogsConfig,
		ADIdentifiers:           config.AdIdentifiers,
		AdvancedADIdentifiers:   advancedAdIdentifiers,
		Provider:                config.Provider,
		ServiceID:               config.ServiceId,
		TaggerEntity:            config.TaggerEntity,
		ClusterCheck:            config.ClusterCheck,
		NodeName:                config.NodeName,
		Source:                  config.Source,
		IgnoreAutodiscoveryTags: config.IgnoreAutodiscoveryTags,
		MetricsExcluded:         config.MetricsExcluded,
		LogsExcluded:            config.LogsExcluded,
	}
}
