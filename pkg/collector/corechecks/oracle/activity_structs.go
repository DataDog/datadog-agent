// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import "database/sql"

// ActivitySnapshot is a payload containing database activity samples. It is parsed from the intake payload.
// easyjson:json
type ActivitySnapshot struct {
	Metadata
	// Tags should be part of the common Metadata struct but because Activity payloads use a string array
	// and samples use a comma-delimited list of tags in a single string, both flavors have to be handled differently
	Tags               []string            `json:"ddtags,omitempty"`
	CollectionInterval float64             `json:"collection_interval,omitempty"`
	OracleActivityRows []OracleActivityRow `json:"oracle_activity,omitempty"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type RowMetadata struct {
	Commands       []string `json:"dd_commands,omitempty"`
	Tables         []string `json:"dd_tables,omitempty"`
	Comments       []string `json:"dd_comments,omitempty"`
	QueryTruncated string   `json:"query_truncated,omitempty"`
}

// Metadata contains the metadata fields common to all events processed
type Metadata struct {
	Timestamp      float64 `json:"timestamp,omitempty"`
	Host           string  `json:"host,omitempty"`
	Source         string  `json:"ddsource,omitempty"`
	DBMType        string  `json:"dbm_type,omitempty"`
	DDAgentVersion string  `json:"ddagentversion,omitempty"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type OracleSQLRow struct {
	SQLID                  string `json:"sql_id,omitempty"`
	ForceMatchingSignature uint64 `json:"force_matching_signature,omitempty"`
	SQLPlanHashValue       uint64 `json:"sql_plan_hash_value,omitempty"`
	SQLExecStart           string `json:"sql_exec_start,omitempty"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type OracleActivityRow struct {
	Now           string `json:"now"`
	SessionID     uint64 `json:"sid,omitempty"`
	SessionSerial uint64 `json:"serial,omitempty"`
	User          string `json:"user,omitempty"`
	Status        string `json:"status"`
	OsUser        string `json:"os_user,omitempty"`
	Process       string `json:"process,omitempty"`
	Client        string `json:"client,omitempty"`
	Port          string `json:"port,omitempty"`
	Program       string `json:"program,omitempty"`
	Type          string `json:"type,omitempty"`
	OracleSQLRow
	Module                string `json:"module,omitempty"`
	Action                string `json:"action,omitempty"`
	ClientInfo            string `json:"client_info,omitempty"`
	LogonTime             string `json:"logon_time,omitempty"`
	ClientIdentifier      string `json:"client_identifier,omitempty"`
	BlockingInstance      uint64 `json:"blocking_instance,omitempty"`
	BlockingSession       uint64 `json:"blocking_session,omitempty"`
	FinalBlockingInstance uint64 `json:"final_blocking_instance,omitempty"`
	FinalBlockingSession  uint64 `json:"final_blocking_session,omitempty"`
	WaitEvent             string `json:"wait_event,omitempty"`
	WaitEventClass        string `json:"wait_event_class,omitempty"`
	WaitTimeMicro         uint64 `json:"wait_time_micro,omitempty"`
	Statement             string `json:"statement,omitempty"`
	PdbName               string `json:"pdb_name,omitempty"`
	CdbName               string `json:"cdb_name,omitempty"`
	QuerySignature        string `json:"query_signature,omitempty"`
	CommandName           string `json:"command_name,omitempty"`
	PreviousSQL           bool   `json:"previous_sql,omitempty"`
	OpFlags               uint64 `json:"op_flags,omitempty"`
	RowMetadata
}

//nolint:revive // TODO(DBM) Fix revive linter
type OracleActivityRowDB struct {
	SampleID                   uint64         `db:"SAMPLE_ID"`
	Now                        string         `db:"NOW"`
	UtcMs                      float64        `db:"UTC_MS"`
	SessionID                  uint64         `db:"SID"`
	SessionSerial              uint64         `db:"SERIAL#"`
	User                       sql.NullString `db:"USERNAME"`
	Status                     string         `db:"STATUS"`
	OsUser                     sql.NullString `db:"OSUSER"`
	Process                    sql.NullString `db:"PROCESS"`
	Client                     sql.NullString `db:"MACHINE"`
	Port                       sql.NullInt64  `db:"PORT"`
	Program                    sql.NullString `db:"PROGRAM"`
	Type                       sql.NullString `db:"TYPE"`
	SQLID                      sql.NullString `db:"SQL_ID"`
	ForceMatchingSignature     *string        `db:"FORCE_MATCHING_SIGNATURE"`
	SQLPlanHashValue           *uint64        `db:"SQL_PLAN_HASH_VALUE"`
	SQLExecStart               sql.NullString `db:"SQL_EXEC_START"`
	SQLAddress                 sql.NullString `db:"SQL_ADDRESS"`
	PrevSQLID                  sql.NullString `db:"PREV_SQL_ID"`
	PrevForceMatchingSignature *string        `db:"PREV_FORCE_MATCHING_SIGNATURE"`
	PrevSQLPlanHashValue       *uint64        `db:"PREV_SQL_PLAN_HASH_VALUE"`
	PrevSQLExecStart           sql.NullString `db:"PREV_SQL_EXEC_START"`
	PrevSQLAddress             sql.NullString `db:"PREV_SQL_ADDRESS"`
	Module                     sql.NullString `db:"MODULE"`
	Action                     sql.NullString `db:"ACTION"`
	ClientInfo                 sql.NullString `db:"CLIENT_INFO"`
	LogonTime                  sql.NullString `db:"LOGON_TIME"`
	ClientIdentifier           sql.NullString `db:"CLIENT_IDENTIFIER"`
	OpFlags                    uint64         `db:"OP_FLAGS"`
	BlockingInstance           *uint64        `db:"BLOCKING_INSTANCE"`
	BlockingSession            *uint64        `db:"BLOCKING_SESSION"`
	FinalBlockingInstance      *uint64        `db:"FINAL_BLOCKING_INSTANCE"`
	FinalBlockingSession       *uint64        `db:"FINAL_BLOCKING_SESSION"`
	WaitEvent                  sql.NullString `db:"EVENT"`
	WaitEventClass             sql.NullString `db:"WAIT_CLASS"`
	WaitTimeMicro              *uint64        `db:"WAIT_TIME_MICRO"`
	Statement                  sql.NullString `db:"SQL_FULLTEXT"`
	PrevSQLFullText            sql.NullString `db:"PREV_SQL_FULLTEXT"`
	PdbName                    sql.NullString `db:"PDB_NAME"`
	CommandName                sql.NullString `db:"COMMAND_NAME"`
}
