// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks
// +build kubeapiserver

package common

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

	v1 "k8s.io/api/core/v1"
)

// ConfigsForService returns the openmetrics configurations for a given service if it matches the AD configuration
func (pc *PrometheusCheck) ConfigsForService(svc *v1.Service) []integration.Config {
	var configs []integration.Config
	namespacedName := fmt.Sprintf("%s/%s", svc.GetNamespace(), svc.GetName())
	if pc.isExcluded(svc.GetAnnotations(), namespacedName) {
		return configs
	}

	instances, found := pc.buildInstances(svc.GetAnnotations(), namespacedName)
	if found {
		serviceID := apiserver.EntityForService(svc)
		configs = append(configs, integration.Config{
			Name:          openmetricsCheckName,
			InitConfig:    integration.Data(openmetricsInitConfig),
			Instances:     instances,
			ClusterCheck:  true,
			Provider:      names.PrometheusServices,
			Source:        "prometheus_services:" + serviceID,
			ADIdentifiers: []string{serviceID},
		})
	}

	return configs
}
