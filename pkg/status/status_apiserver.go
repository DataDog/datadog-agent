// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package status

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"time"
)

func getLeaderElectionDetails() map[string]string {
	leaderElectionStats := make(map[string]string)

	record, err := leaderelection.GetLeaderElectionRecord()
	if err != nil {
		leaderElectionStats["status"] = "Failing"
		leaderElectionStats["error"] = err.Error()
		return leaderElectionStats
	}
	leaderElectionStats["leaderName"] = record.HolderIdentity
	leaderElectionStats["acquiredTime"] = record.AcquireTime.Format(time.RFC1123)
	leaderElectionStats["renewedTime"] = record.RenewTime.Format(time.RFC1123)
	leaderElectionStats["transitions"] = fmt.Sprintf("%d transitions", record.LeaderTransitions)
	leaderElectionStats["status"] = "Running"
	return leaderElectionStats
}

func getDCAStatus() map[string]string {
	clusterAgentDetails := make(map[string]string)

	dcaCl, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		clusterAgentDetails["DetectionError"] = err.Error()
		return clusterAgentDetails
	}
	clusterAgentDetails["Endpoint"] = dcaCl.ClusterAgentAPIEndpoint

	ver, err := dcaCl.GetVersion()
	if err != nil {
		clusterAgentDetails["ConnectionError"] = err.Error()
		return clusterAgentDetails
	}
	clusterAgentDetails["Version"] = ver
	return clusterAgentDetails
}

// GetHorizontalPodAutoscalingStatus fetches the content of the ConfigMap storing the state of the HPA metrics provider
func GetHorizontalPodAutoscalingStatus() map[string]interface{} {
	horizontalPodAutoscalingStatus := make(map[string]interface{})

	apiCl, err := apiserver.GetAPIClient()
	if err != nil {
		horizontalPodAutoscalingStatus["Error"] = err.Error()
		return horizontalPodAutoscalingStatus
	}

	datadogHPAConfigMap := custommetrics.GetConfigmapName()
	horizontalPodAutoscalingStatus["Cmname"] = datadogHPAConfigMap

	store, err := custommetrics.NewConfigMapStore(apiCl.Cl, apiserver.GetResourcesNamespace(), datadogHPAConfigMap)
	externalMetrics, err := store.ListAllExternalMetricValues()
	if err != nil {
		horizontalPodAutoscalingStatus["ErrorStore"] = err.Error()
		return horizontalPodAutoscalingStatus
	}

	horizontalPodAutoscalingStatus["Number"] = len(externalMetrics)
	horizontalPodAutoscalingStatus["Metrics"] = externalMetrics

	return horizontalPodAutoscalingStatus
}
