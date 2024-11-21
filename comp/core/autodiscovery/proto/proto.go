// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proto

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func ProtobufConfigFromAutodiscoveryConfig(config *integration.Config) *core.Config {
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
			KubeEndpoints: &core.KubeNamespacedName{
				Name:      advancedAdIdentifier.KubeEndpoints.Name,
				Namespace: advancedAdIdentifier.KubeEndpoints.Namespace,
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

func AutodiscoveryConfigFromprotobufConfig(config *core.Config) integration.Config {
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
			KubeEndpoints: integration.KubeNamespacedName{
				Name:      advancedAdIdentifier.KubeEndpoints.Name,
				Namespace: advancedAdIdentifier.KubeEndpoints.Namespace,
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
