// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/api/core/v1"
)

/*
Service mapper for a node
{
	"foo": [
		"name": [ "svc1" "svc2" "svc3" ]
	]
}
*/

// mapServices maps each pod (endpoint) to the metadata associated with it.
// It is on a per node basis to avoid mixing up the services pods are actually connected to if all pods of different nodes share a similar subnet, therefore sharing a similar IP.
func (metaBundle *MetadataMapperBundle) mapServices(nodeName string, pods v1.PodList, endpointList v1.EndpointsList) error {
	metaBundle.m.Lock()
	defer metaBundle.m.Unlock()
	ipToEndpoints := make(map[string][]string)    // maps the IP address from an endpoint (pod) to associated services ex: "10.10.1.1" : ["service1","service2"]
	podToIp := make(map[string]map[string]string) // maps the pods of the currently evaluated node to their IP.

	if pods.Items == nil {
		return fmt.Errorf("empty podlist received for nodeName %q", nodeName)
	}
	if nodeName == "" {
		log.Debugf("Service mapper was given an empty node name. Mapping might be incorrect.")
	}

	for _, pod := range pods.Items {
		if pod.Status.PodIP != "" {
			if podToIp[pod.Namespace] == nil {
				podToIp[pod.Namespace] = make(map[string]string)
			}
			podToIp[pod.Namespace][pod.Name] = pod.Status.PodIP
		}
	}
	for _, svc := range endpointList.Items {
		for _, endpointsSubsets := range svc.Subsets {
			if endpointsSubsets.Addresses == nil {
				log.Tracef("A subset of endpoints from %s could not be evaluated", svc.Name)
				continue
			}
			for _, edpt := range endpointsSubsets.Addresses {
				if edpt.NodeName != nil && *edpt.NodeName == nodeName {
					ipToEndpoints[edpt.IP] = append(ipToEndpoints[edpt.IP], svc.Name)
				}
			}
		}
	}
	for ns, pods := range podToIp {
		for name, ip := range pods {
			if svc, found := ipToEndpoints[ip]; found {
				if metaBundle.Services[ns] == nil {
					metaBundle.Services[ns] = make(map[string][]string)
				}
				metaBundle.Services[ns][name] = svc
			}
		}
	}
	log.Tracef("The services matched %q", fmt.Sprintf("%s", metaBundle.Services))
	return nil
}

// ServicesForPod returns the services mapped to a given pod and namespace.
// If nothing is found, the boolean is false. This call is thread-safe.
func (metaBundle *MetadataMapperBundle) ServicesForPod(ns, podName string) ([]string, bool) {
	metaBundle.m.RLock()
	defer metaBundle.m.RUnlock()

	svc, found := metaBundle.Services[ns][podName]
	return svc, found
}
