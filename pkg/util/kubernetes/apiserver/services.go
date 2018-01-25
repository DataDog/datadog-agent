// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package apiserver

//// Covered by test/integration/util/kube_apiserver/services_test.go

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/ericchiang/k8s/api/v1"
)

func (smb *ServiceMapperBundle) mapServices(pods v1.PodList, endpointList v1.EndpointsList) error {
	smb.m.Lock()
	defer smb.m.Unlock()
	ipToEndpoints := make(map[string][]string) // maps an IP address to associated services svc/endpoint
	podToIp := make(map[string]string)

	for _, pod := range pods.Items {
		if *pod.Status.PodIP != "" {
			podToIp[*pod.Metadata.Name] = *pod.Status.PodIP
		}
	}
	log.Infof("this is the podToIp %q", podToIp)
	log.Infof("There are %d services in the cluster\n", len(endpointList.Items))
	for _, svc := range endpointList.Items {
		log.Infof("evanluated svc is %q \n", svc)
		for _, endpointsSubsets := range svc.Subsets {
			log.Infof("evanluated endpointSub is %q \n", *endpointsSubsets)
			for _, edpt := range endpointsSubsets.Addresses {
				ipToEndpoints[*edpt.Ip] = append(ipToEndpoints[*edpt.Ip], *svc.Metadata.Name)
				if edpt.NodeName != nil {
					log.Infof("service for %s is %q\n\n", *svc.Metadata.Name, edpt.NodeName)
				}
			}
		}
	}
	log.Infof("This is ipToEndpoint %q \n", ipToEndpoints)
	for name, ip := range podToIp {
		if svc, found := ipToEndpoints[ip]; found {
			smb.PodNameToServices[name] = svc
		}
	}
	log.Infof("the services matched %q \n", smb.PodNameToServices)
	return nil
}

// GetPodSvcs is used when the API endpoint of the DCA to get the services of a pod is hit.
func GetPodSvcs(nodeName string, podName string) []string {
	smb, found := cache.Cache.Get(nodeName)
	if !found {
		log.Debugf("No metadata was found for the pod %s", podName)
		return nil
	}

	serviceList, _ := smb.(*ServiceMapperBundle).PodNameToServices[podName]
	return serviceList
}
