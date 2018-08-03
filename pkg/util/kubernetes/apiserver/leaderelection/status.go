// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/render"
)

// GetStatus returns status info for leader election.
func GetStatus() map[string]interface{} {
	status := make(map[string]interface{})
	record, err := GetLeaderElectionRecord()
	if err != nil {
		status["status"] = "Failing"
		status["error"] = err.Error()
		return status
	}
	status["leaderName"] = record.HolderIdentity
	status["acquiredTime"] = record.AcquireTime.Format(render.TimeFormat)
	status["renewedTime"] = record.RenewTime.Format(render.TimeFormat)
	status["transitions"] = fmt.Sprintf("%d transitions", record.LeaderTransitions)
	status["status"] = "Running"
	return status
}
