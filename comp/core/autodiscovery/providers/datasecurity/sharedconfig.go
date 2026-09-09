// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

// Types shared by the RC payload (rcconfig.go) and the check instance (checkconfig.go).

// subTask is a single scan sub task, received from RC and forwarded to the check as-is.
type subTask struct {
	SubTaskID      string `json:"sub_task_id"`
	Entity         entity `json:"entity"`
	Query          string `json:"query"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// entity is the data asset (table) a sub task targets. DatabaseHostName resolves the
// local connection; the rest is forwarded to the check.
type entity struct {
	Platform             string `json:"platform"`
	DatabaseClusterName  string `json:"database_cluster_name"`
	DatabaseInstanceName string `json:"database_instance_name"`
	DatabaseHostName     string `json:"database_host_name"`
	Database             string `json:"database"`
	Schema               string `json:"schema"`
	Table                string `json:"table"`
}
