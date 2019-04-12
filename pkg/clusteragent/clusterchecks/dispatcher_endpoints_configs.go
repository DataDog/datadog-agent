// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
)

// GetEndpointsConfigs provides configs of endpoints checks queried by node name
func (d *dispatcher) GetEndpointsConfigs(nodeName string) ([]integration.Config, error) {
	err := d.updateEndpointsChecks()
	if err != nil {
		log.Errorf("Cannot update check maps: %s", err)
		return nil, err
	}
	return d.store.endpointsChecks[nodeName], nil
}

// updateEndpointsChecks updates endpoints configs by listing *v1.Endpoints objects
// that have endpoints checks enabled
func (d *dispatcher) updateEndpointsChecks() error {
	nodesEndpointsMapping := make(map[string][]integration.Config)
	d.store.Lock()
	defer d.store.Unlock()
	for _, epInfo := range d.store.endpointsCache {
		namespace := epInfo.Namespace
		name := epInfo.Name
		if namespace == "" || name == "" {
			log.Warnf("Empty namespace %s or name %s for Endpoints related to %s, configs will not be processed", namespace, name, epInfo.ServiceEntity)
			continue
		}
		kendpoints, err := d.endpointsLister.Endpoints(namespace).Get(name)
		if err != nil {
			log.Errorf("Cannot get Kubernetes endpoints:%s/%s : %s", namespace, name, err)
			return err
		}
		if hasPodRef(kendpoints) {
			newNodesEndpointsMapping := getEndpointsInfo(kendpoints, epInfo)
			nodesEndpointsMapping = unionMaps(nodesEndpointsMapping, newNodesEndpointsMapping)
		}
	}

	d.store.endpointsChecks = nodesEndpointsMapping

	return nil
}

// hasPodRef checks if an Endpoints object is backed by pod
func hasPodRef(kendpoints *v1.Endpoints) bool {
	for i := range kendpoints.Subsets {
		for j := range kendpoints.Subsets[i].Addresses {
			if kendpoints.Subsets[i].Addresses[j].TargetRef == nil {
				return false
			}
			return kendpoints.Subsets[i].Addresses[j].TargetRef.Kind == "Pod"
		}
	}
	return false
}

// getEndpointsInfo returns a map of node names and their correspondent endpoints configs
// from a *v1.Endpoints object and its correspondent service *types.EndpointsInfo object
func getEndpointsInfo(kendpoints *v1.Endpoints, epInfo *types.EndpointsInfo) map[string][]integration.Config {
	nodesEndpointsMapping := make(map[string][]integration.Config)
	for i := range kendpoints.Subsets {
		for j := range kendpoints.Subsets[i].Addresses {
			if nodeName := kendpoints.Subsets[i].Addresses[j].NodeName; nodeName != nil {
				podUID := string(kendpoints.Subsets[i].Addresses[j].TargetRef.UID)
				for _, config := range epInfo.Configs {
					nodesEndpointsMapping[*nodeName] = append(nodesEndpointsMapping[*nodeName], updateADIdentifiers(config, podUID, epInfo.ServiceEntity))
				}
			}
		}
	}
	return nodesEndpointsMapping
}

// updateADIdentifiers generates a config template for an endpoints
// object backed by a given pod, it uses the generic endpoints config got
// from the kube service and add pod entity and kube service entity as AD identifiers
func updateADIdentifiers(config integration.Config, podUID, svcEntity string) integration.Config {
	configCopy := integration.Config{
		Name:            config.Name,
		Instances:       config.Instances,
		InitConfig:      config.InitConfig,
		MetricConfig:    config.MetricConfig,
		LogsConfig:      config.LogsConfig,
		ADIdentifiers:   []string{},
		ClusterCheck:    config.ClusterCheck,
		Provider:        config.Provider,
		EndpointsChecks: config.EndpointsChecks,
	}
	configCopy.ADIdentifiers = append(configCopy.ADIdentifiers, config.ADIdentifiers...)
	configCopy.ADIdentifiers = append(configCopy.ADIdentifiers, getPodEntity(podUID))
	configCopy.ADIdentifiers = append(configCopy.ADIdentifiers, svcEntity)
	return configCopy
}

// unionMaps returns the union of two maps
func unionMaps(first, second map[string][]integration.Config) map[string][]integration.Config {
	for k, v := range second {
		first[k] = append(first[k], v...)
	}
	return first
}
