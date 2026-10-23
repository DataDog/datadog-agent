// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package queryactionsimpl

// DOQueryPayload represents the RC config payload for a database integration instance.
// QuerySpec.DBName is authoritative for execution routing and also selects the local Azure SQL
// Database instance when several databases share a server hostname.
type DOQueryPayload struct {
	ConfigID     string       `json:"config_id"`
	DBIdentifier DBIdentifier `json:"db_identifier"`
	Queries      []QuerySpec  `json:"queries"`
}

// DBIdentifier identifies the integration host to target.
// MySQL and PostgreSQL compare Host with their rendered database identifiers. SQL Server compares
// Host with its configured endpoint, and Azure SQL Database additionally compares every query's
// DBName with the top-level integration database. Type describes the producer's hosting kind and
// is informational.
type DBIdentifier struct {
	Type          string `json:"type"`
	Host          string `json:"host"`
	AgentHostname string `json:"agent_hostname"`
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

// DOTaskPayload represents the RC config payload of a one-off task (kind "task"). A task runs its
// statements once against one database instance and reports each statement's result as a
// do-query-results event. Its RC config ID is do-<platform>-once-<task_id>.
type DOTaskPayload struct {
	ConfigID     string       `json:"config_id"`
	Kind         string       `json:"kind"`
	DBIdentifier DBIdentifier `json:"db_identifier"`
	Task         TaskSpec     `json:"task"`
}

// TaskSpec holds the statements of a one-off task. CreatedAt and ExpiresAt are Unix seconds; a
// task still pending at ExpiresAt is abandoned by the backend, so the agent never starts it late.
type TaskSpec struct {
	TaskID     string          `json:"task_id"`
	CreatedAt  int64           `json:"created_at"`
	ExpiresAt  int64           `json:"expires_at"`
	Statements []TaskStatement `json:"statements"`
}

// TaskStatement is a single SQL statement of a one-off task. The integration runs each statement
// on its own and returns at most MaxRows rows for it, so a failing statement fails alone.
type TaskStatement struct {
	ID             string `json:"id"`
	DBName         string `json:"dbname"`
	Query          string `json:"query"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxRows        int    `json:"max_rows"`
}
