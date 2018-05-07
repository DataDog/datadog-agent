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
)

// GetPodMetadataNames is used when the API endpoint of the DCA to get the metadata of a pod is hit.
func GetPodMetadataNames(nodeName string, podName string) ([]string, error) {
	var metaList []string
	cacheKey := cache.BuildAgentKey(metadataMapperCachePrefix, nodeName)

	metaBundleInterface, found := cache.Cache.Get(cacheKey)
	if !found {
		log.Tracef("no metadata was found for the pod %s on node %s", podName, nodeName)
		return nil, nil
	}

	metaBundle, ok := metaBundleInterface.(*MetadataMapperBundle)
	if !ok {
		return nil, fmt.Errorf("invalid cache format for the cacheKey: %s", cacheKey)
	}
	// The list of metadata collected in the metaBundle is extensible and is handled here.
	// If new cluster level tags need to be collected by the agent, only this needs to be modified.
	serviceList, foundServices := metaBundle.ServicesForPod(podName)
	if !foundServices {
		log.Tracef("no cached services list found for the pod %s on the node %s", podName, nodeName)
		return nil, nil
	}
	log.Debugf("CacheKey: %s, with %d services", cacheKey, len(serviceList))
	for _, s := range serviceList {
		metaList = append(metaList, fmt.Sprintf("kube_service:%s", s))
	}

	return metaList, nil
}
