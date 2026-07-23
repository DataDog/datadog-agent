// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

// DOQueryPayload represents the RC config payload for a database cluster.
// Each config groups all active monitor queries for a single host, with per-query dbname routing.
type DOQueryPayload struct {
	ConfigID     string       `json:"config_id"`
	DBIdentifier DBIdentifier `json:"db_identifier"`
	Queries      []QuerySpec  `json:"queries"`
}

// DBIdentifier identifies a database cluster to target.
// Type describes the hosting kind, such as "self-hosted", "rds" or "azure". Instance matching
// is by host for most types; Azure SQL Database additionally requires Database equality
// because multiple databases share the same server hostname.
type DBIdentifier struct {
	Type          string `json:"type"`
	Host          string `json:"host"`
	AgentHostname string `json:"agent_hostname"`
	Database      string `json:"database,omitempty"`
}

// QuerySpec defines a single monitor query to schedule.
type QuerySpec struct {
	DBName                string                 `json:"dbname,omitempty"`
	MonitorID             int64                  `json:"monitor_id,omitempty"`
	Type                  string                 `json:"type"`
	Query                 string                 `json:"query"`
	IntervalSeconds       int                    `json:"interval_seconds"`
	Schedule              string                 `json:"schedule,omitempty"`
	TimeoutSeconds        int                    `json:"timeout_seconds"`
	Entity                EntityMetadata         `json:"entity"`
	CustomSQLSelectFields *CustomSQLSelectFields `json:"custom_sql_select_fields,omitempty"`
}

// CustomSQLSelectFields identifies the metric config and entity for custom SQL queries,
// since custom SQL cannot encode identity in the column name.
type CustomSQLSelectFields struct {
	MetricConfigID int64  `json:"metric_config_id"`
	EntityID       string `json:"entity_id"`
}

// EntityMetadata describes the data asset a query targets (for lineage/tagging).
type EntityMetadata struct {
	Platform string `json:"platform,omitempty"`
	Account  string `json:"account,omitempty"`
	Database string `json:"database,omitempty"`
	Schema   string `json:"schema,omitempty"`
	Table    string `json:"table,omitempty"`
}
