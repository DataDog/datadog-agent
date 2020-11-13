// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package orchestrator

import (
	"encoding/json"
	"expvar"

	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
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

	// get orchestrator endpoints, check for old keys, looks like this: map[endpoints] = apikey
	newKey := "orchestrator_explorer.orchestrator_additional_endpoints"
	oldKey := "process_config.orchestrator_additional_endpoints"
	if config.Datadog.IsSet(newKey) {
		status["OrchestratorAdditionalEndpoints"] = config.Datadog.GetStringMapStringSlice(newKey)
	} else if config.Datadog.IsSet(oldKey) {
		status["OrchestratorAdditionalEndpoints"] = config.Datadog.GetStringMapStringSlice(oldKey)
	}

	orchestratorEndpoint := config.Datadog.GetString("orchestrator_explorer.orchestrator_dd_url")
	orchestratorOldEndpoint := config.Datadog.GetString("process_config.orchestrator_dd_url")
	if orchestratorOldEndpoint != "" {
		status["OrchestratorEndpoint"] = orchestratorOldEndpoint
	} else if orchestratorEndpoint != "" {
		status["OrchestratorEndpoint"] = orchestratorEndpoint
	}

	// get forwarder stats
	forwarderStatsJSON := []byte(expvar.Get("forwarder").String())
	forwarderStats := make(map[string]interface{})
	json.Unmarshal(forwarderStatsJSON, &forwarderStats) //nolint:errcheck
	transactions := forwarderStats["Transactions"].(map[string]interface{})
	status["Transactions"] = transactions

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
		if value, found := orchestrator.KubernetesResourceCache.Get(BuildStatsKey(node)); found {
			status[node.String()+"sStats"] = value
		}
	}

	// get Leader information
	engine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		status["LeaderError"] = err
	} else {
		status["Leader"] = engine.IsLeader()
		if ip, err := engine.GetLeaderIP(); err == nil {
			status["LeaderIP"] = ip
		} else {
			status["LeaderError"] = err
		}
	}

	// get options
	if config.Datadog.GetBool("orchestrator_explorer.container_scrubbing.enabled") {
		status["ContainerScrubbing"] = "ContainerScrubbing: Enabled"
	}

	return status
}
