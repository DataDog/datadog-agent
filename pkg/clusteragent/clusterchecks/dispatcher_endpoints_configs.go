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

// getEndpointsConfigs provides configs templates of endpoints checks queried by node name.
// Exposed to node agents by the cluster agent api.
func (d *dispatcher) getEndpointsConfigs(nodeName string) ([]integration.Config, error) {
	err := d.updateEndpointsChecks()
	if err != nil {
		log.Errorf("Cannot update check maps: %s", err)
		return nil, err
	}
	return d.store.endpointsChecks[nodeName], nil
}

// updateEndpointsChecks updates stored endpoints configs.
// The function validates cached configs by listing their corresponding
// *v1.Endpoints objects and checking if endpoints are backed by pods,
// if validated, store them as endpoints checks with their correspendent node name.
// Listing the *v1.Endpoints object keeps pods' UIDs updated as they will
// be added as AD identifiers in the endpoints config templates in buildEndpointsChecks.
func (d *dispatcher) updateEndpointsChecks() error {
	endpointsChecks := make(map[string][]integration.Config)
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
			// Only consider endpoints backed by pods
			newEndpointsChecks := buildEndpointsChecks(kendpoints, epInfo)
			endpointsChecks = unionMaps(endpointsChecks, newEndpointsChecks)
		}
	}
	// Store the up-to-date generated endpoints config checks
	d.store.endpointsChecks = endpointsChecks
	return nil
}

// hasPodRef checks if an *v1.Endpoints object is backed by pod.
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

// buildEndpointsChecks returns a map of node names as keys with their
// corresponding endpoints configs as values.
// The function gets node names and pod uids from the *v1.Endpoints object.
// The function adds the corresponding pod uid and service entity as AD identifiers
// to the validated config templates using updateADIdentifiers.
func buildEndpointsChecks(kendpoints *v1.Endpoints, epInfo *types.EndpointsInfo) map[string][]integration.Config {
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

// updateADIdentifiers generates a config template for an endpoints check
// with adding pod entity and kube service entity as AD identifiers.
func updateADIdentifiers(config integration.Config, podUID, svcEntity string) integration.Config {
	updatedConfig := integration.Config{
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
	updatedConfig.ADIdentifiers = append(updatedConfig.ADIdentifiers, config.ADIdentifiers...)
	updatedConfig.ADIdentifiers = append(updatedConfig.ADIdentifiers, getPodEntity(podUID))
	updatedConfig.ADIdentifiers = append(updatedConfig.ADIdentifiers, svcEntity)
	return updatedConfig
}

// unionMaps returns the union of two maps containing endpoints checks.
func unionMaps(first, second map[string][]integration.Config) map[string][]integration.Config {
	for k, v := range second {
		first[k] = append(first[k], v...)
	}
	return first
}
