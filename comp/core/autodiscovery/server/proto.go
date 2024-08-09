// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package server

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func protobufConfigFromAutodiscoveryConfig(config *integration.Config) *pb.Config {
	instances := [][]byte{}

	for _, instance := range config.Instances {
		instances = append(instances, []byte(instance))
	}

	advancedAdIdentifiers := make([]*pb.AdvancedADIdentifier, 0, len(config.AdvancedADIdentifiers))
	for _, advancedAdIdentifier := range config.AdvancedADIdentifiers {
		advancedAdIdentifiers = append(advancedAdIdentifiers, &pb.AdvancedADIdentifier{
			KubeService: &pb.KubeNamespacedName{
				Name:      advancedAdIdentifier.KubeService.Name,
				Namespace: advancedAdIdentifier.KubeService.Namespace,
			},
			KubeEndpoints: &pb.KubeNamespacedName{
				Name:      advancedAdIdentifier.KubeEndpoints.Name,
				Namespace: advancedAdIdentifier.KubeEndpoints.Namespace,
			},
		})
	}

	return &pb.Config{
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
