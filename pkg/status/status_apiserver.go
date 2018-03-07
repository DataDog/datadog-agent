// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package status

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
)

func getLeaderElectionDetails() map[string]string {
	leaderElectionStats := make(map[string]string)

	leaderElectionDetails, err := leaderelection.GetLeaderDetails()
	if err != nil {
		leaderElectionStats["status"] = "Failing"
		leaderElectionStats["error"] = err.Error()
		return leaderElectionStats
	}
	leaderElectionStats["leaderName"] = leaderElectionDetails.HolderIdentity
	leaderElectionStats["acquiredTime"] = leaderElectionDetails.AcquireTime.Format(time.RFC1123)
	leaderElectionStats["renewedTime"] = leaderElectionDetails.RenewTime.Format(time.RFC1123)
	leaderElectionStats["transitions"] = fmt.Sprintf("%d transitions", leaderElectionDetails.LeaderTransitions)
	leaderElectionStats["status"] = "Running"
	return leaderElectionStats
}
