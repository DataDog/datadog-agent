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
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

// GetStatus returns status info for the orchestrator explorer.
func GetStatus(apiCl kubernetes.Interface) map[string]interface{} {
	status := make(map[string]interface{})
	if !config.Datadog.GetBool("orchestrator_explorer.enabled") {
		status["Disabled"] = "The orchestrator explorer is not enabled on the Cluster Agent"
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

	// get orchestrator endpoints, check for old keys
	orchestratorEndpoints := config.Datadog.GetString("orchestrator_explorer.orchestrator_additional_endpoints")
	orchestratorEndpointsOldKey := config.Datadog.GetString("process_config.orchestrator_additional_endpoints")
	if orchestratorEndpointsOldKey != "" {
		status["OrchestratorAdditionalEndpoints"] = orchestratorEndpointsOldKey
	} else if orchestratorEndpoints != "" {
		status["OrchestratorAdditionalEndpoints"] = orchestratorEndpoints
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
	// has Transactions prefix key
	status["forwarderStatsPods"] = transactions["Pods"]
	status["forwarderStatsDeployment"] = transactions["Deployments"]
	status["forwarderStatsReplicaSets"] = transactions["ReplicaSets"]
	status["forwarderStatsServices"] = transactions["Services"]
	status["forwarderStatsNodes"] = transactions["Nodes"]

	// get informer status

	return status
}
