// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/ericchiang/k8s/api/v1"
)

// mapServices maps each pod (endpoint) to the services connected to it.
// It is on a per node basis to avoid mixing up the services pods are actually connected to if all pods of different nodes share a similar subnet, therefore sharing a similar IP.
func (smb *ServiceMapperBundle) mapServices(nodeName string, pods v1.PodList, endpointList v1.EndpointsList) error {
	smb.m.Lock()
	defer smb.m.Unlock()
	ipToEndpoints := make(map[string][]string) // maps the IP address from an endpoint (pod) to associated services ex: "10.10.1.1" : ["service1","service2"]
	podToIp := make(map[string]string)         // maps the pods of the currently evaluated node to their IP.

	if pods.Items == nil {
		return fmt.Errorf("empty podlist received for nodeName %q", nodeName)
	}
	if nodeName == "" {
		log.Debugf("Service mapper was given an empty node name. Mapping might be incorrect.")
	}

	for _, pod := range pods.Items {
		if *pod.Status.PodIP != "" {
			podToIp[*pod.Metadata.Name] = *pod.Status.PodIP
		}
	}
	for _, svc := range endpointList.Items {
		for _, endpointsSubsets := range svc.Subsets {
			if endpointsSubsets.Addresses == nil {
				log.Tracef("A subset of endpoints from %s could not be evaluated", *svc.Metadata.Name)
				continue
			}
			for _, edpt := range endpointsSubsets.Addresses {
				if edpt == nil {
					log.Tracef("An endpoint from %s could not be evaluated", *svc.Metadata.Name)
					continue
				}
				if edpt.NodeName != nil && *edpt.NodeName == nodeName {
					ipToEndpoints[*edpt.Ip] = append(ipToEndpoints[*edpt.Ip], *svc.Metadata.Name)
				}
			}
		}
	}
	for name, ip := range podToIp {
		if svc, found := ipToEndpoints[ip]; found {
			smb.PodNameToServices[name] = svc
		}
	}
	log.Tracef("The services matched %q", fmt.Sprintf("%s", smb.PodNameToServices))
	return nil
}

// GetPodServiceNames is used when the API endpoint of the DCA to get the services of a pod is hit.
func GetPodServiceNames(nodeName string, podName string) []string {
	var serviceList []string
	cacheKey := cache.BuildAgentKey(serviceMapperCachePrefix, nodeName)

	smbInterface, found := cache.Cache.Get(cacheKey)
	if !found {
		log.Debugf("No metadata was found for the pod %s on node %s at the cache key %q", podName, nodeName, cacheKey)
		return serviceList
	}

	smb, ok := smbInterface.(*ServiceMapperBundle)
	if !ok {
		log.Warnf("Invalid cache format at cacheKey: %s", cacheKey)
		return serviceList
	}

	serviceList, found = smb.PodNameToServices[podName]
	if !found {
		log.Debugf("No cached metadata found for the pod %s on the node %s", podName, nodeName)
		return serviceList
	}

	log.Debugf("cacheKey: %s, with %d services", cacheKey, len(serviceList))
	return serviceList
}
