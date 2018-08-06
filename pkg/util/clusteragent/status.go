// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package clusteragent

// GetStatus returns status info for the Datadog Cluster Agent client
func GetStatus() map[string]interface{} {
	status := make(map[string]interface{})

	dcaCl, err := GetClusterAgentClient()
	if err != nil {
		status["DetectionError"] = err.Error()
		return status
	}
	status["Endpoint"] = dcaCl.ClusterAgentAPIEndpoint

	version, err := dcaCl.GetVersion()
	if err != nil {
		status["ConnectionError"] = err.Error()
		return status
	}
	status["Version"] = version
	return status
}
