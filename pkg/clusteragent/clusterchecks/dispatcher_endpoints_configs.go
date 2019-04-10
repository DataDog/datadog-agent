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
	kservices, err := d.listers.ServicesLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Cannot list Kubernetes services: %s", err)
		return err
	}
	d.updateServicesMap(kservices)
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

func (d *dispatcher) getEndpointsInfo() (map[string][]types.EndpointInfo, error) {
	nodesEndpointsMapping := make(map[string][]types.EndpointInfo)
	d.store.RLock()
	defer d.store.Unlock()
	for _, svc := range d.store.services {
		kendpoints, err := d.listers.EndpointsLister.Endpoints(svc.Namespace).Get(svc.Name)
		if err != nil {
			log.Errorf("Cannot get Kubernetes endpoints:%s/%s : %s", svc.Namespace, svc.Name, err)
			return nil, err
		}
		if hasPodRef(kendpoints) {
			newNodesEndpointsMapping := getEndpointInfo(kendpoints, svc)
			nodesEndpointsMapping = unionMaps(nodesEndpointsMapping, newNodesEndpointsMapping)
		}
	}
	return nodesEndpointsMapping, nil
}

func (d *dispatcher) updateServicesMap(kservices []*v1.Service) {
	d.store.Lock()
	defer d.store.Unlock()
	for _, ksvc := range kservices {
		uid := ksvc.ObjectMeta.UID
		_, found := d.store.services[uid]
		if found {
			d.store.services[uid].Namespace = ksvc.Namespace
			d.store.services[uid].Name = ksvc.Name
			d.store.services[uid].ClusterIP = ksvc.Spec.ClusterIP
		}
	}
}

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

func getEndpointInfo(kendpoints *v1.Endpoints, svc *types.Service) map[string][]types.EndpointInfo {
	endpointsInfo := make(map[string][]types.EndpointInfo)
	for i := range kendpoints.Subsets {
		ports := []int32{}
		for j := range kendpoints.Subsets[i].Ports {
			ports = append(ports, kendpoints.Subsets[i].Ports[j].Port)
		}
		for j := range kendpoints.Subsets[i].Addresses {
			if nodeName := kendpoints.Subsets[i].Addresses[j].NodeName; nodeName != nil {
				endpointsInfo[*nodeName] = append(endpointsInfo[*nodeName], types.EndpointInfo{
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
	return endpointsInfo
}

func unionMaps(first, second map[string][]types.EndpointInfo) map[string][]types.EndpointInfo {
	for k, v := range second {
		first[k] = append(first[k], v...)
	}
	return first
}

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
