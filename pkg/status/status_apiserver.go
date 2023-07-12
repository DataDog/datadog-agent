// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package status

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
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
	clusterAgentDetails["Endpoint"] = dcaCl.ClusterAgentAPIEndpoint()

	ver, err := dcaCl.GetVersion()
	if err != nil {
		clusterAgentDetails["ConnectionError"] = err.Error()
		return clusterAgentDetails
	}
	clusterAgentDetails["Version"] = ver.String()
	return clusterAgentDetails
}
