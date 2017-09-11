// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

type instanceStatus struct {
	InstanceName string `json:"instance_name"`
	MCount       int    `json:"metric_count"`
	ScCount      int    `json:"service_check_count"`
	Message      string `json:"initialized_checks"`
}

type checkStatus struct {
	InitializedChecks map[string]instanceStatus `json:"initialized_checks"`
	FailedChecks      map[string]instanceStatus `json:"failed_checks"`
}

type JMXStatus struct {
	ChecksStatus checkStatus `json:"checks"`
	Timestamp    int64       `json:"timestamp"`
}
