// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package providers

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/common/utils"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// PrometheusPodsConfigProvider implements the ConfigProvider interface for prometheus pods.
type PrometheusPodsConfigProvider struct {
	kubelet kubelet.KubeUtilInterface

	checks []*types.PrometheusCheck
}

// NewPrometheusPodsConfigProvider returns a new Prometheus ConfigProvider connected to kubelet.
// Connectivity is not checked at this stage to allow for retries, Collect will do it.
func NewPrometheusPodsConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	checks, err := getPrometheusConfigs()
	if err != nil {
		return nil, err
	}

	p := &PrometheusPodsConfigProvider{
		checks: checks,
	}
	return p, nil
}

// String returns a string representation of the PrometheusPodsConfigProvider
func (p *PrometheusPodsConfigProvider) String() string {
	return names.PrometheusPods
}

// Collect retrieves templates from the kubelet's podlist, builds config objects and returns them
func (p *PrometheusPodsConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	var err error
	if p.kubelet == nil {
		p.kubelet, err = kubelet.GetKubeUtil()
		if err != nil {
			return []integration.Config{}, err
		}
	}

	pods, err := p.kubelet.GetLocalPodList(ctx)
	if err != nil {
		return []integration.Config{}, err
	}

	return p.parsePodlist(pods), nil
}

// IsUpToDate always return false to poll new data from kubelet
func (p *PrometheusPodsConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	return false, nil
}

// parsePodlist searches for pods that match the AD configuration
func (p *PrometheusPodsConfigProvider) parsePodlist(podlist []*kubelet.Pod) []integration.Config {
	var configs []integration.Config
	for _, pod := range podlist {
		for _, check := range p.checks {
			configs = append(configs, utils.ConfigsForPod(check, pod)...)
		}
	}
	return configs
}

func init() {
	RegisterProvider(names.PrometheusPodsRegisterName, NewPrometheusPodsConfigProvider)
}

// GetConfigErrors is not implemented for the PrometheusPodsConfigProvider
func (p *PrometheusPodsConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}
