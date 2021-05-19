// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package orchestrator

import (
	"encoding/json"
	"expvar"
	"fmt"

	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

// GetStatus returns status info for the orchestrator explorer.
func GetStatus(apiCl kubernetes.Interface) map[string]interface{} {
	status := make(map[string]interface{})
	if !config.Datadog.GetBool("orchestrator_explorer.enabled") {
		status["Disabled"] = "The orchestrator explorer is not enabled on the Cluster Agent"
		return status
	}

	if !config.Datadog.GetBool("leader_election") {
		status["Disabled"] = "Leader election is not enabled on the Cluster Agent. The orchestrator explorer needs leader election for resource collection."
		return status
	}

	// get cluster uid
	clusterID, err := common.GetOrCreateClusterID(apiCl.CoreV1())
	if err != nil {
		status["ClusterIDError"] = err.Error()
	} else {
		status["ClusterID"] = clusterID
	}

	setClusterName(status)
	setCollectionIsWorking(status)

	// get orchestrator endpoints
	endpoints := map[string][]string{}
	orchestratorCfg := orchcfg.NewDefaultOrchestratorConfig()
	err = orchestratorCfg.Load()
	if err == nil {
		// obfuscate the api keys
		for _, endpoint := range orchestratorCfg.OrchestratorEndpoints {
			endpointStr := endpoint.Endpoint.String()
			if len(endpoint.APIKey) > 5 {
				endpoints[endpointStr] = append(endpoints[endpointStr], endpoint.APIKey[len(endpoint.APIKey)-5:])
			}
		}
	}
	status["OrchestratorEndpoints"] = endpoints

	// get cache size
	status["CacheNumber"] = orchestrator.KubernetesResourceCache.ItemCount()

	// get cache hits
	cacheHitsJSON := []byte(expvar.Get("orchestrator-cache").String())
	cacheHits := make(map[string]interface{})
	json.Unmarshal(cacheHitsJSON, &cacheHits) //nolint:errcheck
	status["CacheHits"] = cacheHits

	// get cache Miss
	cacheMissJSON := []byte(expvar.Get("orchestrator-sends").String())
	cacheMiss := make(map[string]interface{})
	json.Unmarshal(cacheMissJSON, &cacheMiss) //nolint:errcheck
	status["CacheMiss"] = cacheMiss

	// get cache efficiency
	for _, node := range orchestrator.NodeTypes() {
		if value, found := orchestrator.KubernetesResourceCache.Get(orchestrator.BuildStatsKey(node)); found {
			status[node.String()+"sStats"] = value
		}
	}

	// get Leader information
	engine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		status["LeaderError"] = err
	} else {
		status["Leader"] = engine.IsLeader()
		status["LeaderName"] = engine.GetLeader()
	}

	// get options
	if config.Datadog.GetBool("orchestrator_explorer.container_scrubbing.enabled") {
		status["ContainerScrubbing"] = "Container scrubbing: enabled"
	}

	return status
}

func setClusterName(status map[string]interface{}) {
	errorMsg := "No cluster name was detected. This means resource collection will not work."

	hostname, err := util.GetHostname()
	if err != nil {
		status["ClusterNameError"] = fmt.Sprintf("Error detecting cluster name: %s.\n%s", err.Error(), errorMsg)
	} else {
		if cName := clustername.GetClusterName(hostname); cName != "" {
			status["ClusterName"] = cName
		} else {
			status["ClusterName"] = errorMsg
		}
	}
}

// setCollectionIsWorking checks whether collection is running by checking telemetry/cache data
func setCollectionIsWorking(status map[string]interface{}) {
	c := orchestrator.KubernetesResourceCache.ItemCount()
	if c > 0 {
		status["CollectionWorking"] = "The collection is at least partially running since the cache has been populated."
	} else {
		status["CollectionWorking"] = "The collection has not run successfully yet since the cache is empty."
	}
}
