// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package doqueryactionsimpl

// DOQueryPayload represents the RC config payload for a single DB instance.
// Each config contains the full set of active monitor queries for that instance.
type DOQueryPayload struct {
	DOQueryAction bool         `json:"do_query_action"`
	ConfigID      string       `json:"config_id"`
	DBIdentifier  DBIdentifier `json:"db_identifier"`
	Queries       []QuerySpec  `json:"queries"`
}

// DBIdentifier identifies a database instance to target
type DBIdentifier struct {
	Type                 string `json:"type"` // "self-hosted", "rds", "aurora"
	Host                 string `json:"host,omitempty"`
	Port                 int    `json:"port,omitempty"`
	DBName               string `json:"dbname,omitempty"`
	DBInstanceIdentifier string `json:"dbinstanceidentifier,omitempty"`
}

// QuerySpec defines a single monitor query to schedule
type QuerySpec struct {
	MonitorID       int64          `json:"monitor_id"`
	Query           string         `json:"query"`
	IntervalSeconds int            `json:"interval_seconds"`
	TimeoutSeconds  int            `json:"timeout_seconds"`
	Entity          EntityMetadata `json:"entity"`
}

// EntityMetadata describes the data asset a query targets (for lineage/tagging)
type EntityMetadata struct {
	Platform       string `json:"platform"`
	AccountID      string `json:"account_id"`
	Database       string `json:"database"`
	Schema         string `json:"schema"`
	Table          string `json:"table"`
	MetricConfigID int64  `json:"metric_config_id"`
	Measure        string `json:"measure"`
}
