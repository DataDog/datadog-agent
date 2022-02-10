// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"fmt"
)

// GetProcessAgentStats returns the status command of the process-agent
func GetProcessAgentStatus() map[string]interface{} {
	//net.SetSystemProbePath(socketPath)
	//probeUtil, err := net.GetRemoteSystemProbeUtil()

	//if err != nil {
	//	return map[string]interface{}{
	//		"Errors": fmt.Sprintf("%v", err),
	//	}
	//}

	processAgentStatus := map[string]interface{}{
		"Errors": fmt.Sprintf("Testing process-agent status"),
	}

	//systemProbeDetails, err := probeUtil.GetStats()
	//if err != nil {
	//	return map[string]interface{}{
	//		"Errors": fmt.Sprintf("issue querying stats from system probe: %v", err),
	//	}
	//}

	return processAgentStatus
}
