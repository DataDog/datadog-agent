// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package orchestrator

import (
	"encoding/json"
	"expvar"

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
	// get cluster name
	hostname, err := util.GetHostname()
	if err != nil {
		status["ClusterNameError"] = err.Error()
	} else {
		status["ClusterName"] = clustername.GetClusterName(hostname)
	}

	// get orchestrator endpoints
	endpoints := map[string]string{}
	orchestratorCfg := orchcfg.NewDefaultOrchestratorConfig()
	err = orchestratorCfg.LoadYamlConfig(config.Datadog.ConfigFileUsed())
	if err == nil {
		// obfuscate the api keys
		for _, endpoint := range orchestratorCfg.OrchestratorEndpoints {
			if len(endpoint.APIKey) > 5 {
				endpoints[endpoint.Endpoint.String()] = endpoint.APIKey[len(endpoint.APIKey)-5:]
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
