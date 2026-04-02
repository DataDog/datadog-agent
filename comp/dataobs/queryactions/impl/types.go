// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

// DOQueryPayload represents the RC config payload for a single DB instance.
// Each config contains the full set of active monitor queries for that instance.
type DOQueryPayload struct {
	ConfigID     string       `json:"config_id"`
	DBIdentifier DBIdentifier `json:"db_identifier"`
	Queries      []QuerySpec  `json:"queries"`
}

// DBIdentifier identifies a database instance to target.
// Type describes the hosting kind (e.g. "self-hosted", "rds"). It is stored
// for informational purposes; instance matching is by host and dbname only.
type DBIdentifier struct {
	Type   string `json:"type"`
	Host   string `json:"host"`
	DBName string `json:"dbname"`
	// AgentHostname is deserialized from RC but is not currently used for filtering.
	// The RC backend is expected to route each config only to the agent whose hostname matches.
	// TODO: If RC delivery becomes broadcast (all agents receive all configs), filter here by
	// comparing AgentHostname against the current agent's hostname to prevent duplicate checks
	// in multi-agent deployments where multiple agents monitor the same postgres host.
	AgentHostname string `json:"agent_hostname"`
}

// QuerySpec defines a single monitor query to schedule.
type QuerySpec struct {
	MonitorID             int64                  `json:"monitor_id,omitempty"`
	Type                  string                 `json:"type"`
	Query                 string                 `json:"query"`
	IntervalSeconds       int                    `json:"interval_seconds"`
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
