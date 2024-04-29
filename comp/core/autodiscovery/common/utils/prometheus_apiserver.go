// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package utils

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	v1 "k8s.io/api/core/v1"
)

// Kind and prefix in Kubernetes
const (
	kubePodKind   = "Pod"
	KubePodPrefix = "kubernetes_pod://"
)

// ConfigsForService returns the openmetrics configurations for a given service if it matches the AD configuration
func ConfigsForService(pc *types.PrometheusCheck, svc *v1.Service) []integration.Config {
	var configs []integration.Config
	namespacedName := fmt.Sprintf("%s/%s", svc.GetNamespace(), svc.GetName())

	// Ignore headless services because we can't resolve the IP.
	// Ref: https://kubernetes.io/docs/concepts/services-networking/service/#headless-services
	if svc.Spec.ClusterIP == "None" {
		log.Debugf("ignoring Prometheus-annotated headless service: %s", namespacedName)
		return configs
	}

	instances, found := buildInstances(pc, svc.GetAnnotations(), namespacedName)
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

// ConfigsForServiceEndpoints returns the openmetrics configurations for a given endpoints if it matches the AD
// configuration for related service
func ConfigsForServiceEndpoints(pc *types.PrometheusCheck, svc *v1.Service, ep *v1.Endpoints) []integration.Config {
	var configs []integration.Config
	namespacedName := fmt.Sprintf("%s/%s", svc.GetNamespace(), svc.GetName())
	instances, found := buildInstances(pc, svc.GetAnnotations(), namespacedName)
	if found {
		for _, subset := range ep.Subsets {
			for _, address := range subset.Addresses {
				endpointsID := apiserver.EntityForEndpoints(ep.GetNamespace(), ep.GetName(), address.IP)

				epConfig := integration.Config{
					ServiceID:     endpointsID,
					Name:          openmetricsCheckName,
					InitConfig:    integration.Data(openmetricsInitConfig),
					Instances:     instances,
					ClusterCheck:  true,
					Provider:      names.PrometheusServices,
					Source:        "prometheus_services:" + endpointsID,
					ADIdentifiers: []string{endpointsID},
				}

				ResolveEndpointConfigAuto(&epConfig, address)
				configs = append(configs, epConfig)
			}
		}
	}

	return configs
}

// ResolveEndpointConfigAuto automatically resolves endpoint pod and node information if available
func ResolveEndpointConfigAuto(conf *integration.Config, addr v1.EndpointAddress) {
	log.Debugf("using 'auto' resolve for config: %s, entity: %s", conf.Name, conf.ServiceID)
	if targetRef := addr.TargetRef; targetRef != nil && targetRef.Kind == kubePodKind {
		// The endpoint is backed by a pod.
		// We add the pod uid as AD identifiers so the check can get the pod tags.
		podUID := string(targetRef.UID)
		conf.ADIdentifiers = append(conf.ADIdentifiers, getPodEntity(podUID))
		if nodeName := addr.NodeName; nodeName != nil {
			// Set the node name to schedule the endpoint check on the correct node.
			// This field needs to be set only when the endpoint is backed by a pod.
			conf.NodeName = *nodeName
		}
	}
}

// getPodEntity returns pod entity
func getPodEntity(podUID string) string {
	return KubePodPrefix + podUID
}
