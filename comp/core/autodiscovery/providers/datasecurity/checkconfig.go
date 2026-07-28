// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import "encoding/json"

// checkInstance is the instance config handed to the datasecurity Rust check,
// mirroring its `CheckConfig`. scanning_rules (dd-sds rules) are passed through verbatim.
type checkInstance struct {
	MinCollectionInterval int               `json:"min_collection_interval"`
	TaskID                string            `json:"task_id"`
	ScanningRules         []json.RawMessage `json:"scanning_rules"`
	ScanData              []checkSubTask    `json:"scan_data"`
}

// checkSubTask is a single sub task as consumed by the datasecurity Rust check.
// It is the RC subTask plus the connection resolved locally from the matching
// integration.
type checkSubTask struct {
	subTask
	Connection connection `json:"connection"`
}

// connection holds the database connection parameters resolved locally from the
// matching integration. Mirrors the check's `Connection` struct.
type connection struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	DBName   string `json:"dbname"`
	Username string `json:"username"`
	Password string `json:"password"`
}
