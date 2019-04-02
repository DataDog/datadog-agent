// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"bytes"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (d *dispatcher) GetEndpointsConfigs(nodeName string) ([]integration.Config, error) {
	err := d.updateCheckMaps()
	if err != nil {
		log.Errorf("Cannot update check maps: %s", err)
		return nil, err
	}
	return d.store.endpointsChecks[nodeName], nil
}

func (d *dispatcher) updateCheckMaps() error {
	kservices, err := d.listers.ServicesLister.List(labels.Everything())
	if err != nil {
		log.Errorf("Cannot list Kubernetes services: %s", err)
		return err
	}
	d.updateServiceChecksMap(kservices)
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
		for _, endpoint := range endpoints {
			newEndpointsChecks[nodeName] = append(newEndpointsChecks[nodeName], resolveToConfig(endpoint))
		}
	}
	d.store.Lock()
	d.store.endpointsChecks = newEndpointsChecks
	d.store.Unlock()
	return nil
}

func (d *dispatcher) getEndpointsInfo() (map[string][]types.EndpointInfo, error) {
	endpointsInfo := make(map[string][]types.EndpointInfo)
	d.store.Lock()
	defer d.store.Unlock()
	for _, svc := range d.store.serviceChecks {
		kendpoints, err := d.listers.EndpointsLister.Endpoints(svc.Namespace).Get(svc.Name)
		if err != nil {
			log.Errorf("Cannot get Kubernetes endpoints:%s/%s : %s", svc.Namespace, svc.Name, err)
			return nil, err
		}
		if hasPodRef(kendpoints) {
			newEndpointsInfo := getEndpointInfo(kendpoints, svc)
			endpointsInfo = mergeEndpointsInfo(endpointsInfo, newEndpointsInfo)
		}
	}
	return endpointsInfo, nil
}

func (d *dispatcher) updateServiceChecksMap(kservices []*v1.Service) {
	d.store.Lock()
	defer d.store.Unlock()
	for _, ksvc := range kservices {
		uid := ksvc.ObjectMeta.UID
		_, found := d.store.serviceChecks[uid]
		if found {
			d.store.serviceChecks[uid].Namespace = ksvc.Namespace
			d.store.serviceChecks[uid].Name = ksvc.Name
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
	ports := []int32{}
	for i := range kendpoints.Subsets {
		for j := range kendpoints.Subsets[i].Ports {
			ports = append(ports, kendpoints.Subsets[i].Ports[j].Port)
		}
		for j := range kendpoints.Subsets[i].Addresses {
			if kendpoints.Subsets[i].Addresses[j].NodeName != nil {
				nodeName := *kendpoints.Subsets[i].Addresses[j].NodeName
				endpointsInfo[nodeName] = append(endpointsInfo[nodeName], types.EndpointInfo{
					PodUID:     kendpoints.Subsets[i].Addresses[j].TargetRef.UID,
					IP:         kendpoints.Subsets[i].Addresses[j].IP,
					Ports:      ports,
					CheckName:  svc.CheckName,
					Instances:  svc.Instances,
					InitConfig: svc.InitConfig,
				})
			}
		}
	}
	return endpointsInfo
}

func mergeEndpointsInfo(first, second map[string][]types.EndpointInfo) map[string][]types.EndpointInfo {
	for k, v := range second {
		first[k] = append(first[k], v...)
	}
	return first
}

func resolveToConfig(info types.EndpointInfo) integration.Config {
	config := integration.Config{
		Name:          info.CheckName,
		Instances:     info.Instances,
		InitConfig:    info.InitConfig,
		ADIdentifiers: []string{string(info.PodUID)},
		ClusterCheck:  false,
		CreationTime:  integration.Before,
	}
	for i := 0; i < len(config.Instances); i++ {
		vars := config.GetTemplateVariablesForInstance(i)
		for _, v := range vars {
			switch string(v.Name) {
			case "host":
				config.InitConfig = bytes.Replace(config.InitConfig, v.Raw, []byte(info.IP), -1)
				config.Instances[i] = bytes.Replace(config.Instances[i], v.Raw, []byte(info.IP), -1)
			case "port":
				if len(info.Ports) > 0 {
					port := strconv.Itoa(int(info.Ports[0]))
					config.InitConfig = bytes.Replace(config.InitConfig, v.Raw, []byte(port), -1)
					config.Instances[i] = bytes.Replace(config.Instances[i], v.Raw, []byte(port), -1)
				}
			}
		}
	}
	return config
}
