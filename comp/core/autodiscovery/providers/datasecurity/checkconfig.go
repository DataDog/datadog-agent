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
//
// SSL mirrors each integration's own `ssl` key: a string SSL mode for postgres
// (e.g. "verify-full") or a mysqlSSL object for mysql. It is left nil (and thus
// omitted) when the integration configures no TLS, matching the integrations.
type connection struct {
	Host        string `json:"host"`
	Sock        string `json:"sock,omitempty"`
	Port        int    `json:"port"`
	DBName      string `json:"dbname"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	SSL         any    `json:"ssl,omitempty"`
	SSLRootCert string `json:"ssl_root_cert,omitempty"`
	SSLCert     string `json:"ssl_cert,omitempty"`
	SSLKey      string `json:"ssl_key,omitempty"`
	SSLPassword string `json:"ssl_password,omitempty"`
}

// mysqlSSL mirrors the MySQL integration's `ssl` section.
type mysqlSSL struct {
	CA            string `json:"ca,omitempty"`
	Cert          string `json:"cert,omitempty"`
	Key           string `json:"key,omitempty"`
	CheckHostname *bool  `json:"check_hostname,omitempty"`
}
