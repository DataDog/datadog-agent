// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DistributedChecksProvider implements the ConfigProvider interface for prometheus pods.
type DistributedChecksProvider struct {
	checks []*types.PrometheusCheck
}

// NewDistributedChecksProvider returns a new Prometheus ConfigProvider connected to kubelet.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewDistributedChecksProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	checks, err := getPrometheusConfigs()
	if err != nil {
		return nil, err
	}

	p := &DistributedChecksProvider{
		checks: checks,
	}
	return p, nil
}

// String returns a string representation of the DistributedChecksProvider
func (p *DistributedChecksProvider) String() string {
	return names.PrometheusPods
}

// Collect retrieves templates from the kubelet's podlist, builds config objects and returns them
func (p *DistributedChecksProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	log.Info("[DistributedChecks] Collect")
	configs, _, err := ReadConfigFiles(GetAll)
	if err != nil {
		return nil, log.Errorf("read config files: %s", err)
	}
	var newConfig []integration.Config
	for _, config := range configs {
		if config.IsTemplate() {
			continue
		}

		if config.Name == "snmp" {
			log.Infof("[DistributedChecks] config %s (cluster_check=%t): \n%s", config.Name, config.ClusterCheck, config.String())
			config.ClusterCheck = false
			config.InitConfig.SetField("distributed_check", true)
			log.Infof("[DistributedChecks] config after %s (cluster_check=%t): \n%s", config.Name, config.ClusterCheck, config.String())
			newConfig = append(newConfig, config)
		}
	}
	//
	//instances := []integration.Config{
	//	{
	//		Name:       "snmp",
	//		Instances:  []integration.Data{integration.Data(`{"ip_address":"1.2.3.4", "community_string": "public"}`)},
	//		InitConfig: integration.Data("{}"),
	//	},
	//}
	return newConfig, nil
}

// IsUpToDate always return false to poll new data from kubelet
func (p *DistributedChecksProvider) IsUpToDate(ctx context.Context) (bool, error) {
	return false, nil
}

func init() {
	RegisterProvider(names.DistributedChecksRegisterName, NewDistributedChecksProvider)
}

// GetConfigErrors is not implemented for the DistributedChecksProvider
func (p *DistributedChecksProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
