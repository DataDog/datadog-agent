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

// ServicesMapper maps pod names to the names of the services targeting the pod
// keyed by the namespace a pod belongs to. This data structure allows for O(1)
// lookups of services given a namespace and pod name.
//
// The data is stored in the following schema:
// {
// 	"namespace": {
// 		"pod": [ "svc1", "svc2", "svc3" ]
// 	}
// }
type ServicesMapper map[string]map[string][]string

func (m ServicesMapper) Get(ns, podName string) ([]string, error) {
	pods, ok := m[ns]
	if !ok {
		return nil, fmt.Errorf("no mapping for namespace %s", ns)
	}
	svcs, ok := pods[podName]
	if !ok {
		return nil, fmt.Errorf("no mapping for pod %s in namespace %s", podName, ns)
	}
	return svcs, nil
}

func (m ServicesMapper) Set(ns, podName string, svcs []string) {
	if _, ok := m[ns]; !ok {
		m[ns] = make(map[string][]string)
	}
	m[ns][podName] = svcs
}

func (m ServicesMapper) Map(nodeName string, pods v1.PodList, endpointList v1.EndpointsList) error {
	ipToEndpoints := make(map[string][]string)    // maps the IP address from an endpoint (pod) to associated services ex: "10.10.1.1" : ["service1","service2"]
	podToIp := make(map[string]map[string]string) // maps pod names to its IP address keyed by the namespace a pod belongs to

	if pods.Items == nil {
		return fmt.Errorf("empty podlist received for nodeName %q", nodeName)
	}
	if nodeName == "" {
		log.Debugf("Service mapper was given an empty node name. Mapping might be incorrect.")
	}

	for _, pod := range pods.Items {
		if pod.Status.PodIP == "" {
			log.Debugf("PodIP is empty, ignoring pod %s in namespace %s", pod.Name, pod.Namespace)
			continue
		}
		if _, ok := podToIp[pod.Namespace]; !ok {
			podToIp[pod.Namespace] = make(map[string]string)
		}
		podToIp[pod.Namespace][pod.Name] = pod.Status.PodIP
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
			if svcs, found := ipToEndpoints[ip]; found {
				m.Set(ns, name, svcs)
			}
		}
	}
	return nil
}

// mapServices maps each pod (endpoint) to the metadata associated with it.
// It is on a per node basis to avoid mixing up the services pods are actually connected to if all pods of different nodes share a similar subnet, therefore sharing a similar IP.
func (metaBundle *MetadataMapperBundle) mapServices(nodeName string, pods v1.PodList, endpointList v1.EndpointsList) error {
	metaBundle.m.Lock()
	defer metaBundle.m.Unlock()

	if err := metaBundle.Services.Map(nodeName, pods, endpointList); err != nil {
		return err
	}
	log.Tracef("The services matched %q", fmt.Sprintf("%s", metaBundle.Services))
	return nil
}

// ServicesForPod returns the services mapped to a given pod and namespace.
// If nothing is found, the boolean is false. This call is thread-safe.
func (metaBundle *MetadataMapperBundle) ServicesForPod(ns, podName string) ([]string, bool) {
	metaBundle.m.RLock()
	defer metaBundle.m.RUnlock()

	svcs, err := metaBundle.Services.Get(ns, podName)
	if err != nil {
		log.Errorf("could not get services: %s", err)
		return nil, false
	}
	return svcs, true
}
