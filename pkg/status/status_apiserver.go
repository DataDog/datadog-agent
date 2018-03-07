// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package status

import (
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"
)

// GetDCAStatus grabs the status from expvar and puts it into a map
func GetDCAStatus() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	stats, err := expvarStats(stats)
	if err != nil {
		log.Errorf("Error Getting ExpVar Stats: %v", err)
	}
	stats["config"] = getPartialConfig()

	stats["version"] = version.AgentVersion
	stats["pid"] = os.Getpid()
	hostname, err := util.GetHostname()
	if err != nil {
		log.Errorf("Error grabbing hostname for status: %v", err)
		stats["metadata"] = host.GetPayloadFromCache("unknown")
	} else {
		stats["metadata"] = host.GetPayloadFromCache(hostname)
	}
	now := time.Now()
	stats["time"] = now.Format(timeFormat)
	stats["leaderelection"] = getLeaderElectionDetails()

	return stats, nil
}

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
