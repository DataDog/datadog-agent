// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// KubeServiceFileConfigProvider generates cluster checks from check configurations defined in files.
type KubeServiceFileConfigProvider struct {
}

// NewKubeServiceFileConfigProvider returns a new KubeServiceFileConfigProvider
func NewKubeServiceFileConfigProvider(*config.ConfigurationProviders) (ConfigProvider, error) {
	return &KubeServiceFileConfigProvider{}, nil
}

// Collect returns the check configurations defined in Yaml files.
// Only configs with advanced AD identifiers targeting kubernetes services are handled by this collector.
func (c *KubeServiceFileConfigProvider) Collect(ctx context.Context) ([]integration.Config, error) {
	configs, _, err := ReadConfigFiles(WithAdvancedADOnly)
	if err != nil {
		return nil, err
	}

	return toKubernetesServiceChecks(configs), nil
}

// IsUpToDate is not implemented for the file providers as the files are not meant to change.
func (c *KubeServiceFileConfigProvider) IsUpToDate(ctx context.Context) (bool, error) {
	return false, nil
}

// String returns a string representation of the KubeServiceFileConfigProvider.
func (c *KubeServiceFileConfigProvider) String() string {
	return names.KubeServicesFile
}

// GetConfigErrors is not implemented for the KubeServiceFileConfigProvider.
func (c *KubeServiceFileConfigProvider) GetConfigErrors() map[string]ErrorMsgSet {
	return make(map[string]ErrorMsgSet)
}

// toKubernetesServiceChecks generates integration configs to target
// kubernetes services (cluster checks) based on advanced AD identifiers.
func toKubernetesServiceChecks(configs []integration.Config) []integration.Config {
	k8sServiceChecks := []integration.Config{}
	for i, config := range configs {
		if len(config.AdvancedADIdentifiers) > 0 {
			adIdentifiers := toServiceADIdentifiers(config.AdvancedADIdentifiers)
			if len(adIdentifiers) == 0 {
				continue
			}

			configs[i].ADIdentifiers = adIdentifiers
			configs[i].AdvancedADIdentifiers = nil
			configs[i].Provider = names.KubeServicesFile
			configs[i].ClusterCheck = true

			k8sServiceChecks = append(k8sServiceChecks, configs[i])
		}
	}

	return k8sServiceChecks
}

// toServiceADIdentifiers converts advanced AD identifiers into AD identifiers
func toServiceADIdentifiers(advancedIDs []integration.AdvancedADIdentifier) []string {
	adIdentifiers := []string{}
	for _, advancedID := range advancedIDs {
		if !advancedID.KubeService.IsEmpty() {
			adIdentifiers = append(adIdentifiers, apiserver.EntityForServiceWithNames(advancedID.KubeService.Namespace, advancedID.KubeService.Name))
		}
	}

	return adIdentifiers
}

func init() {
	RegisterProvider(names.KubeServicesFileRegisterName, NewKubeServiceFileConfigProvider)
}
