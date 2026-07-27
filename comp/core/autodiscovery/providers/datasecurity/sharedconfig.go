// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

// These types are shared by the RC payload (rcconfig.go) and the check instance
// (checkconfig.go): the sub task shape received from RC is the same one forwarded
// to the check (checkSubTask embeds subTask and only adds the resolved connection).

// subTask is a single scan sub task. It is received from RC and forwarded to the
// check as-is (see checkSubTask, which embeds it and adds the resolved connection).
type subTask struct {
	SubTaskID      string `json:"sub_task_id"`
	Entity         entity `json:"entity"`
	Query          string `json:"query"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// entity is the logical data asset (table) a sub task targets, as received from RC.
// database_host_name is used to resolve the local connection; it is forwarded to
// the check as-is (the check ignores it).
type entity struct {
	Platform             string `json:"platform"`
	DatabaseClusterName  string `json:"database_cluster_name"`
	DatabaseInstanceName string `json:"database_instance_name"`
	DatabaseHostName     string `json:"database_host_name"`
	Database             string `json:"database"`
	Schema               string `json:"schema"`
	Table                string `json:"table"`
}
