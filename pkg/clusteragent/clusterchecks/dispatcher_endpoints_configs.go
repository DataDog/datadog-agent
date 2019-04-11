// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (d *dispatcher) GetEndpointsConfigs(nodeName string) ([]integration.Config, error) {
	err := d.updateStore()
	if err != nil {
		log.Errorf("Cannot update check maps: %s", err)
		return nil, err
	}
	return d.store.endpointsChecks[nodeName], nil
}

func (d *dispatcher) updateStore() error {
	d.store.RLock()
	updateNeeded := d.store.services.UpdateNeeded
	d.store.RUnlock()

	// Update services cache only when needed
	if updateNeeded {
		kservices, err := d.listers.ServicesLister.List(labels.Everything())
		if err != nil {
			log.Errorf("Cannot list Kubernetes services: %s", err)
			return err
		}
		d.updateServices(kservices)
	}

	// Update endpoints cache
	endpointsInfo, err := d.getEndpointsInfo()
	if err != nil {
		log.Errorf("Cannot get endpoints info: %s", err)
		return err
	}
	err = d.updateEndpointsChecksMap(endpointsInfo)
	if err != nil {
		log.Errorf("Cannot update endpoints checks: %s", err)
		return err
	}
	return nil
}

// updateEndpointsChecksMap updates the stored endpoints data
func (d *dispatcher) updateEndpointsChecksMap(endpointsInfo map[string][]types.EndpointInfo) error {
	newEndpointsChecks := make(map[string][]integration.Config)
	for nodeName, endpoints := range endpointsInfo {
		resolvedConfigs := []integration.Config{}
		for _, endpoint := range endpoints {
			resolvedConfigs = append(resolvedConfigs, resolveToConfig(endpoint))
		}
		newEndpointsChecks[nodeName] = resolvedConfigs
	}
	d.store.Lock()
	d.store.endpointsChecks = newEndpointsChecks
	d.store.Unlock()
	return nil
}

// getEndpointsInfo returns a map of node names and their correspondent endpoints
// the function uses the cached services to list Endpoints objects by Namespace and Name
func (d *dispatcher) getEndpointsInfo() (map[string][]types.EndpointInfo, error) {
	nodesEndpointsMapping := make(map[string][]types.EndpointInfo)
	d.store.RLock()
	defer d.store.RUnlock()
	for _, svc := range d.store.services.Service {
		kendpoints, err := d.listers.EndpointsLister.Endpoints(svc.Namespace).Get(svc.Name)
		if err != nil {
			log.Errorf("Cannot get Kubernetes endpoints:%s/%s : %s", svc.Namespace, svc.Name, err)
			return nil, err
		}
		if hasPodRef(kendpoints) {
			newNodesEndpointsMapping := getOneEndpointInfo(kendpoints, svc)
			nodesEndpointsMapping = unionMaps(nodesEndpointsMapping, newNodesEndpointsMapping)
		}
	}
	return nodesEndpointsMapping, nil
}

// updateServices update the current services cache by adding Namespaces, Names, and ClusterIPs
// from kube *v1.Service objects
// the function should be called when d.store.services.UpdateNeeded is set to true
func (d *dispatcher) updateServices(kservices []*v1.Service) {
	d.store.Lock()
	defer d.store.Unlock()
	for _, ksvc := range kservices {
		uid := ksvc.ObjectMeta.UID
		if _, found := d.store.services.Service[uid]; found {
			d.store.services.Service[uid].Namespace = ksvc.Namespace
			d.store.services.Service[uid].Name = ksvc.Name
			d.store.services.Service[uid].ClusterIP = ksvc.Spec.ClusterIP
		}
	}
	// Cache update is done, set UpdateNeeded to false
	// UpdateNeeded will be set to true when a new service check is scheduled
	d.store.services.UpdateNeeded = false
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

// getOneEndpointInfo returns a map of node names and their correspondent endpoints info
// from a *v1.Endpoints object and its correspondent service *types.ServiceInfo object
// *types.ServiceInfo is used to enrich the EndpointInfo with the service config
func getOneEndpointInfo(kendpoints *v1.Endpoints, svc *types.ServiceInfo) map[string][]types.EndpointInfo {
	nodesEndpointsMapping := make(map[string][]types.EndpointInfo)
	for i := range kendpoints.Subsets {
		ports := []int32{}
		for j := range kendpoints.Subsets[i].Ports {
			ports = append(ports, kendpoints.Subsets[i].Ports[j].Port)
		}
		for j := range kendpoints.Subsets[i].Addresses {
			if nodeName := kendpoints.Subsets[i].Addresses[j].NodeName; nodeName != nil {
				nodesEndpointsMapping[*nodeName] = append(nodesEndpointsMapping[*nodeName], types.EndpointInfo{
					PodUID:        kendpoints.Subsets[i].Addresses[j].TargetRef.UID,
					IP:            kendpoints.Subsets[i].Addresses[j].IP,
					Ports:         ports,
					CheckName:     svc.CheckName,
					Instances:     svc.Instances,
					InitConfig:    svc.InitConfig,
					ClusterIP:     svc.ClusterIP,
					ServiceEntity: svc.Entity,
				})
			}
		}
	}
	return nodesEndpointsMapping
}

// unionMaps returns the union of two maps
func unionMaps(first, second map[string][]types.EndpointInfo) map[string][]types.EndpointInfo {
	for k, v := range second {
		first[k] = append(first[k], v...)
	}
	return first
}

// resolveToConfig creates and integration.Config from types.EndpointInfo
// since info in EndpointInfo is retrieved from a ServiceInfo object
// the function replaces ClusterIP by the actual endpoint IP in the
// InitConfig and Instances fields
func resolveToConfig(info types.EndpointInfo) integration.Config {
	entity := getEndpointsEntity(string(info.PodUID))
	config := integration.Config{
		Name:          info.CheckName,
		Entity:        entity,
		Instances:     make([]integration.Data, len(info.Instances)),
		InitConfig:    make(integration.Data, len(info.InitConfig)),
		ADIdentifiers: []string{entity, info.ServiceEntity},
		ClusterCheck:  false,
		CreationTime:  integration.Before,
	}
	copy(config.InitConfig, info.InitConfig)
	copy(config.Instances, info.Instances)
	for i := 0; i < len(config.Instances); i++ {
		config.InitConfig = bytes.Replace(config.InitConfig, []byte(info.ClusterIP), []byte(info.IP), -1)
		config.Instances[i] = bytes.Replace(config.Instances[i], []byte(info.ClusterIP), []byte(info.IP), -1)
	}
	return config
}
