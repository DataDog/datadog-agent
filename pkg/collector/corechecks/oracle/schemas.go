// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

//nolint:unused // Declarations in this collector are wired by later PRs in the stack.
package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultSchemaPayloadChunkSize = 1000

// A snapshot is scoped to one container. Its payloads share collection_started_at, and only
// the final payload carries collection_payloads_count.

const schemaOwnersQuery = `SELECT con_id, username, user_id FROM cdb_users WHERE oracle_maintained = 'N'`

// Oracle object IDs are unique only within a container.
const objectIDsQuery = `SELECT con_id, owner, object_name, object_id FROM cdb_objects
WHERE object_type = 'TABLE' AND /*RELATIONS*/`

// CDB_* scans must be owner-scoped; unfiltered scans can consume tens of millions of buffer gets.
// Object table metadata comes from cdb_object_tables, while its columns remain in cdb_tab_cols;
// cdb_object_tables also lacks CLUSTERING and READ_ONLY.
//
// Relation identities are selected before columns so a table is never split.
const tableIdentitiesQueryTemplate = `SELECT con_id, owner, table_name FROM (
	SELECT con_id, owner, table_name FROM (
		SELECT t.con_id, t.owner, t.table_name
		FROM cdb_tables t
		WHERE t.con_id = /*CON_ID*/
			AND t.nested = 'NO'
			AND t.secondary = 'N'
			AND NVL(t.dropped, 'NO') = 'NO'
			AND (t.iot_type IS NULL OR t.iot_type = 'IOT')
			AND t.table_name NOT LIKE 'BIN$%'
			AND t.owner IN (/*OWNERS*/)
			AND NOT EXISTS (
				SELECT 1 FROM cdb_object_tables ot
				WHERE ot.con_id = t.con_id AND ot.owner = t.owner AND ot.table_name = t.table_name
			)
			/*TABLE_FILTERS*/
		UNION ALL
		SELECT t.con_id, t.owner, t.table_name
		FROM cdb_object_tables t
		WHERE t.con_id = /*CON_ID*/
			AND t.nested = 'NO'
			AND t.secondary = 'N'
			AND NVL(t.dropped, 'NO') = 'NO'
			AND (t.iot_type IS NULL OR t.iot_type = 'IOT')
			AND t.table_name NOT LIKE 'BIN$%'
			AND t.owner IN (/*OWNERS*/)
			/*TABLE_FILTERS*/
		ORDER BY owner, table_name
	)
) WHERE ROWNUM <= /*IDENTITY_LIMIT*/`

const schemasQueryTemplate = `WITH ranked_columns AS (
	SELECT c.con_id, c.owner, c.table_name, c.column_name, c.column_id, c.internal_column_id,
		c.virtual_column, c.hidden_column, c.data_type, c.data_type_owner, c.data_type_mod,
		c.data_length, c.char_length, c.data_precision, c.data_scale, c.char_used, c.nullable,
		CAST(NULL AS VARCHAR2(4000)) AS data_default_vc,
		ROW_NUMBER() OVER (PARTITION BY c.con_id, c.owner, c.table_name ORDER BY c.internal_column_id) AS col_rn,
		COUNT(*) OVER (PARTITION BY c.con_id, c.owner, c.table_name) AS total_columns
	FROM cdb_tab_cols c
	WHERE c.owner IN (/*OWNERS*/)
		AND /*COLUMN_RELATIONS*/
		AND NOT (c.hidden_column = 'YES' AND c.user_generated = 'NO')
)
SELECT
	t.con_id,
	t.owner,
	t.table_name,
	t.temporary,
	NVL(t.duration, '-') AS duration,
	t.external,
	NVL(t.iot_type, '-') AS iot_type,
	NVL(t.partitioned, 'NO') AS partitioned,
	NVL(t.cluster_name, '-') AS cluster_name,
	NVL(t.clustering, 'NO') AS clustering,
	NVL(t.read_only, 'NO') AS read_only,
	'-' AS object_type_owner,
	'-' AS object_type,
	CAST(NULL AS NUMBER) AS total_tables,
	CASE WHEN c.column_name IS NULL THEN 0 ELSE 1 END AS column_present,
	NVL(c.column_name, '-') AS column_name,
	c.column_id,
	c.internal_column_id,
	NVL(c.virtual_column, '-') AS virtual_column,
	NVL(c.hidden_column, '-') AS hidden_column,
	c.data_type,
	c.data_type_owner,
	c.data_type_mod,
	c.data_length,
	c.char_length,
	c.data_precision,
	c.data_scale,
	NVL(c.char_used, '-') AS char_used,
	NVL(c.nullable, '-') AS nullable,
	c.data_default_vc,
	c.total_columns
FROM cdb_tables t
LEFT JOIN ranked_columns c
	ON c.con_id = t.con_id AND c.owner = t.owner AND c.table_name = t.table_name
WHERE /*RELATIONS*/
	AND NOT EXISTS (
		SELECT 1 FROM cdb_object_tables ot
		WHERE ot.con_id = t.con_id AND ot.owner = t.owner AND ot.table_name = t.table_name
	)
	AND (c.col_rn <= /*MAX_COLUMNS*/ OR c.col_rn IS NULL)
UNION ALL
SELECT
	t.con_id,
	t.owner,
	t.table_name,
	t.temporary,
	NVL(t.duration, '-') AS duration,
	t.external,
	NVL(t.iot_type, '-') AS iot_type,
	NVL(t.partitioned, 'NO') AS partitioned,
	NVL(t.cluster_name, '-') AS cluster_name,
	'NO' AS clustering,
	'NO' AS read_only,
	NVL(t.table_type_owner, '-') AS object_type_owner,
	NVL(t.table_type, '-') AS object_type,
	CAST(NULL AS NUMBER) AS total_tables,
	CASE WHEN c.column_name IS NULL THEN 0 ELSE 1 END AS column_present,
	NVL(c.column_name, '-') AS column_name,
	c.column_id,
	c.internal_column_id,
	NVL(c.virtual_column, '-') AS virtual_column,
	NVL(c.hidden_column, '-') AS hidden_column,
	c.data_type,
	c.data_type_owner,
	c.data_type_mod,
	c.data_length,
	c.char_length,
	c.data_precision,
	c.data_scale,
	NVL(c.char_used, '-') AS char_used,
	NVL(c.nullable, '-') AS nullable,
	c.data_default_vc,
	c.total_columns
FROM cdb_object_tables t
LEFT JOIN ranked_columns c
	ON c.con_id = t.con_id AND c.owner = t.owner AND c.table_name = t.table_name
WHERE /*RELATIONS*/ AND (c.col_rn <= /*MAX_COLUMNS*/ OR c.col_rn IS NULL)
ORDER BY con_id, owner, table_name, internal_column_id`

// These 21c+ views are optional and separately granted.
const blockchainTablesQuery = `SELECT con_id, schema_name, table_name, row_retention, row_retention_locked,
	table_inactivity_retention, hash_algorithm, table_version
FROM cdb_blockchain_tables WHERE /*RELATIONS*/`

const immutableTablesQuery = `SELECT con_id, schema_name, table_name, row_retention, row_retention_locked,
	table_inactivity_retention
FROM cdb_immutable_tables WHERE /*RELATIONS*/`

const partTablesQuery = `SELECT pt.con_id, pt.owner, pt.table_name, pt.partitioning_type,
	pt.subpartitioning_type, pt.partition_count, pkc.column_name
FROM cdb_part_tables pt
LEFT JOIN cdb_part_key_columns pkc
	ON pkc.con_id = pt.con_id AND pkc.owner = pt.owner AND pkc.name = pt.table_name
	AND pkc.object_type = 'TABLE'
WHERE /*RELATIONS*/
ORDER BY pt.con_id, pt.owner, pt.table_name, pkc.column_position`

const tableCommentsQuery = `SELECT con_id, owner, table_name, comments
FROM cdb_tab_comments WHERE comments IS NOT NULL AND /*RELATIONS*/`

const columnCommentsQuery = `SELECT con_id, owner, table_name, column_name, comments
FROM cdb_col_comments WHERE comments IS NOT NULL AND /*RELATIONS*/`

const columnDefaultsQuery = `SELECT c.con_id, c.owner, c.table_name, c.column_name, /*DEFAULT_COL*/ AS data_default
FROM cdb_tab_cols c WHERE /*RELATIONS*/`

// Oracle represents function-based index expressions as hidden SYS_NC%$ virtual columns.
// CDB_IND_COLUMNS exposes only the generated name, while CDB_TAB_COLS exposes the expression.
// CDB_IND_EXPRESSIONS cannot be used because its CDB_ variant omits COLUMN_EXPRESSION.
const indexesQuery = `SELECT i.con_id, i.table_owner, i.table_name, i.owner, i.index_name, i.uniqueness, i.index_type,
	ic.column_name, /*EXPRESSION_COL*/ AS column_expression
FROM cdb_indexes i
JOIN cdb_ind_columns ic
	ON ic.con_id = i.con_id AND ic.index_owner = i.owner AND ic.index_name = i.index_name
LEFT JOIN cdb_tab_cols tc
	ON tc.con_id = ic.con_id AND tc.owner = ic.table_owner AND tc.table_name = ic.table_name
	AND tc.column_name = ic.column_name AND ic.column_name LIKE 'SYS\_NC%' ESCAPE '\'
WHERE /*RELATIONS*/
ORDER BY i.con_id, i.table_owner, i.table_name, i.owner, i.index_name, ic.column_position`

// generated = 'USER NAME' excludes Oracle's system-generated NOT NULL checks, which would
// duplicate column nullability metadata.
const constraintsQuery = `SELECT c.con_id, c.owner, c.table_name, c.constraint_name, c.constraint_type,
	NVL(c.r_owner, '-') AS r_owner, NVL(c.r_constraint_name, '-') AS r_constraint_name, cc.column_name,
	/*CONDITION_COL*/ AS search_condition
FROM cdb_constraints c
JOIN cdb_cons_columns cc
	ON cc.con_id = c.con_id AND cc.owner = c.owner AND cc.constraint_name = c.constraint_name
WHERE (c.constraint_type IN ('P', 'U', 'R') OR (c.constraint_type = 'C' AND c.generated = 'USER NAME'))
	AND /*RELATIONS*/
ORDER BY c.con_id, c.owner, c.table_name, c.constraint_name, cc.position`

const referencedConstraintsQuery = `SELECT c.con_id, c.owner, c.constraint_name, c.table_name, cc.column_name
FROM cdb_constraints c
JOIN cdb_cons_columns cc
	ON cc.con_id = c.con_id AND cc.owner = c.owner AND cc.constraint_name = c.constraint_name
WHERE /*RELATIONS*/
ORDER BY c.con_id, c.owner, c.constraint_name, cc.position`

const externalTablesQuery = `SELECT et.con_id, et.owner, et.table_name, et.type_name,
	NVL(et.default_directory_name, '-'), NVL(el.directory_name, '-'), el.location
FROM cdb_external_tables et
LEFT JOIN cdb_external_locations el
	ON el.con_id = et.con_id AND el.owner = et.owner AND el.table_name = et.table_name
WHERE /*RELATIONS*/
ORDER BY et.con_id, et.owner, et.table_name`

// Materialized views also appear in CDB_TABLES and need separate classification.
const mviewsQuery = `SELECT con_id, owner, mview_name, NVL(refresh_mode, '-'), NVL(refresh_method, '-'),
	NVL(staleness, '-'), last_refresh_date
FROM cdb_mviews WHERE /*RELATIONS*/`

// CDB_* views omit closed or restricted PDBs; those are not empty databases.
const containerNamesQuery = `SELECT con_id, name FROM v$containers
WHERE open_mode IN ('READ WRITE', 'READ ONLY') AND NVL(restricted, 'NO') = 'NO'
AND EXISTS (SELECT 1 FROM cdb_users u WHERE u.con_id = v$containers.con_id AND u.username = 'SYS')`

// total_views is aliased to total_tables for the shared row scanner.
const viewsQueryTemplate = `WITH ranked_views AS (
	SELECT v.con_id, v.owner, v.view_name,
		ROW_NUMBER() OVER (PARTITION BY v.con_id ORDER BY v.owner, v.view_name) AS rn,
		COUNT(*) OVER (PARTITION BY v.con_id) AS total_views
	FROM cdb_views v
	WHERE v.owner IN (/*OWNERS*/)
		/*TABLE_FILTERS*/
),
ranked_columns AS (
	SELECT c.con_id, c.owner, c.table_name, c.column_name, c.column_id, c.internal_column_id,
		c.virtual_column, c.hidden_column, c.data_type, c.data_type_owner, c.data_type_mod,
		c.data_length, c.char_length, c.data_precision, c.data_scale, c.char_used, c.nullable,
		CAST(NULL AS VARCHAR2(4000)) AS data_default_vc,
		ROW_NUMBER() OVER (PARTITION BY c.con_id, c.owner, c.table_name ORDER BY c.internal_column_id) AS col_rn,
		COUNT(*) OVER (PARTITION BY c.con_id, c.owner, c.table_name) AS total_columns
	FROM cdb_tab_cols c
	WHERE c.owner IN (/*OWNERS*/)
		AND NOT (c.hidden_column = 'YES' AND c.user_generated = 'NO')
)
SELECT
	rv.con_id,
	rv.owner,
	rv.view_name AS table_name,
	'N' AS temporary,
	'-' AS duration,
	'NO' AS external,
	'-' AS iot_type,
	'NO' AS partitioned,
	'-' AS cluster_name,
	'NO' AS clustering,
	'NO' AS read_only,
	c.column_name,
	c.column_id,
	c.virtual_column,
	c.hidden_column,
	c.data_type,
	c.data_type_owner,
	c.data_type_mod,
	c.data_length,
	c.char_length,
	c.data_precision,
	c.data_scale,
	NVL(c.char_used, '-') AS char_used,
	c.nullable,
	c.data_default_vc,
	rv.total_views AS total_tables,
	c.total_columns
FROM ranked_views rv
JOIN ranked_columns c
	ON c.con_id = rv.con_id AND c.owner = rv.owner AND c.table_name = rv.view_name
WHERE rv.rn <= /*MAX_VIEWS*/
	AND c.col_rn <= /*MAX_COLUMNS*/
ORDER BY rv.con_id, rv.owner, rv.view_name, c.internal_column_id`

const viewDefinitionsQuery = `SELECT con_id, owner, view_name, text_vc
FROM cdb_views WHERE /*RELATIONS*/`

const viewObjectsQuery = `SELECT con_id, owner, object_name, object_id, created, last_ddl_time
FROM cdb_objects WHERE object_type = 'VIEW' AND /*RELATIONS*/`

// ORA-01795 limits an IN list to 1000 expressions.
const (
	maxSchemaOwners            = 1000
	maxSchemaRelationsPerQuery = 1000
	schemaRelationPageSize     = 100
)

const (
	oracleErrorInvalidIdentifier       = "ORA-00904"
	oracleErrorTableOrViewDoesNotExist = "ORA-00942"
)

var schemaOwnerPattern = regexp.MustCompile(`^[A-Z0-9_$#]+$`)

func escapeSQLLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func regexSQLClauses(column string, include, exclude []string) string {
	var b strings.Builder
	for _, p := range exclude {
		b.WriteString(" AND NOT REGEXP_LIKE(" + column + ", '" + escapeSQLLiteral(p) + "', 'i')")
	}
	if len(include) > 0 {
		parts := make([]string, len(include))
		for i, p := range include {
			parts[i] = "REGEXP_LIKE(" + column + ", '" + escapeSQLLiteral(p) + "', 'i')"
		}
		b.WriteString(" AND (" + strings.Join(parts, " OR ") + ")")
	}
	return b.String()
}

type schemaRowDB struct {
	ConID            int64          `db:"CON_ID"`
	Owner            string         `db:"OWNER"`
	TableName        string         `db:"TABLE_NAME"`
	Temporary        string         `db:"TEMPORARY"`
	Duration         string         `db:"DURATION"`
	External         string         `db:"EXTERNAL"`
	IotType          string         `db:"IOT_TYPE"`
	Partitioned      string         `db:"PARTITIONED"`
	ClusterName      string         `db:"CLUSTER_NAME"`
	Clustering       string         `db:"CLUSTERING"`
	ReadOnly         string         `db:"READ_ONLY"`
	ObjectTypeOwner  string         `db:"OBJECT_TYPE_OWNER"`
	ObjectType       string         `db:"OBJECT_TYPE"`
	TotalTables      sql.NullInt64  `db:"TOTAL_TABLES"`
	TotalColumns     sql.NullInt64  `db:"TOTAL_COLUMNS"`
	ColumnPresent    sql.NullInt64  `db:"COLUMN_PRESENT"`
	ColumnName       string         `db:"COLUMN_NAME"`
	InternalColumnID sql.NullInt64  `db:"INTERNAL_COLUMN_ID"`
	DataType         sql.NullString `db:"DATA_TYPE"`
	DataTypeOwner    sql.NullString `db:"DATA_TYPE_OWNER"`
	DataTypeMod      sql.NullString `db:"DATA_TYPE_MOD"`
	CharLength       sql.NullInt64  `db:"CHAR_LENGTH"`
	ColumnID         sql.NullInt64  `db:"COLUMN_ID"`
	VirtualColumn    string         `db:"VIRTUAL_COLUMN"`
	HiddenColumn     string         `db:"HIDDEN_COLUMN"`
	DataLength       sql.NullInt64  `db:"DATA_LENGTH"`
	DataPrecision    sql.NullInt64  `db:"DATA_PRECISION"`
	DataScale        sql.NullInt64  `db:"DATA_SCALE"`
	CharUsed         string         `db:"CHAR_USED"`
	Nullable         string         `db:"NULLABLE"`
	DataDefault      sql.NullString `db:"DATA_DEFAULT_VC"`
}

type schemaColumn struct {
	Name      string `json:"name"`
	DataType  string `json:"data_type"`
	Nullable  bool   `json:"nullable"`
	Default   string `json:"default,omitempty"`
	Comment   string `json:"comment,omitempty"`
	Virtual   bool   `json:"virtual,omitempty"`
	Invisible bool   `json:"invisible,omitempty"`
}

type temporaryDetail struct {
	Scope string `json:"scope"`
}

type partitionDetail struct {
	PartitionKey      string `json:"partition_key,omitempty"`
	NumPartitions     int64  `json:"num_partitions"`
	PartitioningType  string `json:"partitioning_type,omitempty"`
	SubpartitionsType string `json:"subpartitioning_type,omitempty"`
}

type retentionDetail struct {
	RowRetentionDays        *int64 `json:"row_retention_days,omitempty"`
	RowRetentionLocked      bool   `json:"row_retention_locked"`
	InactivityRetentionDays *int64 `json:"inactivity_retention_days,omitempty"`
	HashAlgorithm           string `json:"hash_algorithm,omitempty"`
	TableVersion            string `json:"table_version,omitempty"`
}

type indexKeyPart struct {
	Column     string `json:"name,omitempty"`
	Expression string `json:"expression,omitempty"`
}

type indexInfo struct {
	owner   string
	Name    string         `json:"name"`
	Unique  bool           `json:"is_unique"`
	Type    string         `json:"index_type,omitempty"`
	Columns []indexKeyPart `json:"columns,omitempty"`
}

type constraintInfo struct {
	Name                 string   `json:"name"`
	Type                 string   `json:"type"`
	Columns              []string `json:"columns,omitempty"`
	Condition            string   `json:"condition,omitempty"`
	ReferencedOwner      string   `json:"referenced_owner,omitempty"`
	ReferencedTable      string   `json:"referenced_table,omitempty"`
	ReferencedColumns    []string `json:"referenced_columns,omitempty"`
	ReferencedConstraint string   `json:"referenced_constraint,omitempty"`
	referencedKey        *constraintKey
	referencedName       string
}

type mviewDetail struct {
	RefreshMode     string `json:"refresh_mode,omitempty"`
	RefreshMethod   string `json:"refresh_method,omitempty"`
	Staleness       string `json:"staleness,omitempty"`
	LastRefreshDate string `json:"last_refresh_date,omitempty"`
}

type externalDetail struct {
	AccessDriver string   `json:"access_driver,omitempty"`
	Directory    string   `json:"default_directory,omitempty"`
	Locations    []string `json:"locations,omitempty"`
}

type objectTypeDetail struct {
	TypeOwner string `json:"type_owner,omitempty"`
	TypeName  string `json:"type_name"`
}

type tableDetails struct {
	ID             string
	Comment        string
	ColumnComments map[string]string
	ColumnDefaults map[string]string
	Indexes        []*indexInfo
	Constraints    []*constraintInfo
	External       *externalDetail
	Mview          *mviewDetail
	Temporary      *temporaryDetail
	Partitioned    *partitionDetail
	Blockchain     *retentionDetail
	Immutable      *retentionDetail
}

type schemaTable struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name"`
	Owner       string            `json:"owner"`
	TableType   string            `json:"table_type"`
	Properties  []string          `json:"table_properties,omitempty"`
	Temporary   *temporaryDetail  `json:"temporary_details,omitempty"`
	Partitioned *partitionDetail  `json:"partitioned_details,omitempty"`
	Blockchain  *retentionDetail  `json:"blockchain_details,omitempty"`
	Immutable   *retentionDetail  `json:"immutable_details,omitempty"`
	ObjectType  *objectTypeDetail `json:"object_type_details,omitempty"`
	Comment     string            `json:"comment,omitempty"`
	External    *externalDetail   `json:"external_details,omitempty"`
	Mview       *mviewDetail      `json:"materialized_view_details,omitempty"`
	Indexes     []*indexInfo      `json:"indexes,omitempty"`
	Constraints []*constraintInfo `json:"constraints,omitempty"`
	Columns     []schemaColumn    `json:"columns"`
}

type schemaObject struct {
	ID     string         `json:"id,omitempty"`
	Name   string         `json:"name"`
	Owner  string         `json:"owner"`
	Tables []*schemaTable `json:"tables"`
	Views  []*viewObject  `json:"views,omitempty"`
}

type viewObject struct {
	ID         string         `json:"id,omitempty"`
	Name       string         `json:"name"`
	Owner      string         `json:"owner"`
	Definition string         `json:"definition,omitempty"`
	Comment    string         `json:"comment,omitempty"`
	CreateDate string         `json:"create_date,omitempty"`
	ModifyDate string         `json:"modify_date,omitempty"`
	Columns    []schemaColumn `json:"columns"`
}

type viewDetails struct {
	ID         string
	Definition string
	Comment    string
	CreateDate string
	ModifyDate string
}

type containerObject struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Schemas []*schemaObject `json:"schemas"`
}

type schemaEvent struct {
	Host                    string            `json:"host"`
	DatabaseInstance        string            `json:"database_instance"`
	AgentVersion            string            `json:"agent_version"`
	Dbms                    string            `json:"dbms"`
	Kind                    string            `json:"kind"`
	CollectionInterval      int64             `json:"collection_interval"`
	DbmsVersion             string            `json:"dbms_version"`
	Tags                    []string          `json:"tags"`
	Timestamp               float64           `json:"timestamp"`
	CollectionStartedAt     int64             `json:"collection_started_at"`
	CollectionPayloadsCount int               `json:"collection_payloads_count,omitempty"`
	Truncated               bool              `json:"truncated,omitempty"`
	Metadata                []containerObject `json:"metadata"`
}

type payloadEmitter func(payload []byte)
type schemaEventEmitter func(event schemaEvent)

type schemaSnapshot struct {
	startedAt int64
	count     int
	pending   *schemaEvent
}

type schemaSnapshotCoordinator struct {
	emit      payloadEmitter
	validate  func(string) error
	snapshots map[string]*schemaSnapshot
	order     []string
	err       error
}

func newSchemaSnapshotCoordinator(emit payloadEmitter) *schemaSnapshotCoordinator {
	return &schemaSnapshotCoordinator{emit: emit, snapshots: make(map[string]*schemaSnapshot)}
}

func (c *schemaSnapshotCoordinator) add(event schemaEvent) {
	if c.err != nil {
		return
	}
	if len(event.Metadata) != 1 {
		c.err = fmt.Errorf("schema event contains %d containers", len(event.Metadata))
		return
	}

	containerID := event.Metadata[0].ID
	snapshot := c.snapshots[containerID]
	if snapshot == nil {
		snapshot = &schemaSnapshot{startedAt: event.CollectionStartedAt}
		c.snapshots[containerID] = snapshot
		c.order = append(c.order, containerID)
	}
	if snapshot.pending != nil {
		c.emitEvent(*snapshot.pending)
	}
	event.CollectionStartedAt = snapshot.startedAt
	event.CollectionPayloadsCount = 0
	snapshot.count++
	snapshot.pending = &event
}

func (c *schemaSnapshotCoordinator) emitEvent(event schemaEvent) {
	if c.err != nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		c.err = err
		return
	}
	c.emit(payload)
}

func (c *schemaSnapshotCoordinator) complete() error {
	var err error
	for _, containerID := range c.order {
		err = errors.Join(err, c.completeContainerID(containerID))
	}
	return errors.Join(err, c.err)
}

func (c *schemaSnapshotCoordinator) completeContainer(conID int64) error {
	return errors.Join(c.completeContainerID(strconv.FormatInt(conID, 10)), c.err)
}

func (c *schemaSnapshotCoordinator) completeContainerID(containerID string) error {
	snapshot := c.snapshots[containerID]
	if snapshot == nil || snapshot.pending == nil {
		return nil
	}
	if c.validate != nil {
		if err := c.validate(containerID); err != nil {
			c.emitEvent(*snapshot.pending)
			snapshot.pending = nil
			return err
		}
	}
	snapshot.pending.CollectionPayloadsCount = snapshot.count
	c.emitEvent(*snapshot.pending)
	snapshot.pending = nil
	return c.err
}

func (c *schemaSnapshotCoordinator) hasContainer(conID int64) bool {
	_, ok := c.snapshots[strconv.FormatInt(conID, 10)]
	return ok
}

func schemaPayloadEmitter(c *Check, emit payloadEmitter) schemaEventEmitter {
	return func(event schemaEvent) {
		payload, err := json.Marshal(event)
		if err != nil {
			log.Errorf("%s failed to marshal schema payload: %s", c.logPrompt, err)
			return
		}
		emit(payload)
	}
}

func emitSchemaSnapshotEvents(events []schemaEvent, complete bool, emit payloadEmitter) error {
	coordinator := newSchemaSnapshotCoordinator(emit)
	for _, event := range events {
		coordinator.add(event)
	}
	if complete {
		return coordinator.complete()
	}
	return coordinator.err
}

type tableKey struct {
	conID int64
	owner string
	table string
}

type constraintKey struct {
	conID int64
	owner string
	name  string
}

type schemaCollector struct {
	check               *Check
	kind                string
	emit                schemaEventEmitter
	payloadChunkSize    int
	details             map[tableKey]*tableDetails
	owners              map[ownerKey]string
	containers          map[int64]string
	started             map[int64]struct{}
	truncatedContainers map[int64]struct{}

	conID       int64
	conName     string
	startedAt   int64
	payloads    int
	schemas     []*schemaObject
	tableCount  int
	tablesTotal int

	truncated bool

	currentSchema *schemaObject
	currentTable  *schemaTable
}

func newSchemaCollector(c *Check, emit payloadEmitter, details map[tableKey]*tableDetails, owners map[ownerKey]string, containers map[int64]string) *schemaCollector {
	return newSchemaEventCollector(c, schemaPayloadEmitter(c, emit), details, owners, containers)
}

func newSchemaEventCollector(c *Check, emit schemaEventEmitter, details map[tableKey]*tableDetails, owners map[ownerKey]string, containers map[int64]string) *schemaCollector {
	payloadChunkSize := c.schemaPayloadChunkSize
	if payloadChunkSize <= 0 {
		payloadChunkSize = defaultSchemaPayloadChunkSize
	}
	return &schemaCollector{check: c, kind: "oracle_databases", emit: emit, payloadChunkSize: payloadChunkSize, details: details, owners: owners, containers: containers, conID: -1, started: make(map[int64]struct{})}
}

func (s *schemaCollector) startContainer(conID int64) {
	if s.conID != -1 {
		s.maybeFlush(true)
	}
	s.conID = conID
	s.started[conID] = struct{}{}
	if name, ok := s.containers[conID]; ok {
		s.conName = s.check.getFullPDBName(name)
	} else {
		s.conName = s.check.getFullPDBName(strconv.FormatInt(conID, 10))
	}
	s.startedAt = s.check.nextSnapshotID()
	s.payloads = 0
	_, s.truncated = s.truncatedContainers[conID]
	s.reset()
}

// collection_started_at must stay unique when collections share a clock millisecond.
func (c *Check) nextSnapshotID() int64 {
	now := c.clock.Now().UnixMilli()
	if now <= c.lastSnapshotID {
		now = c.lastSnapshotID + 1
	}
	c.lastSnapshotID = now
	return now
}

func (s *schemaCollector) reset() {
	s.schemas = nil
	s.tableCount = 0
	s.currentSchema = nil
	s.currentTable = nil
}

func (s *schemaCollector) baseEvent() schemaEvent {
	return schemaEvent{
		Host:                s.check.dbHostname,
		DatabaseInstance:    s.check.dbInstanceIdentifier,
		AgentVersion:        s.check.agentVersion,
		Dbms:                "oracle",
		Kind:                s.kind,
		CollectionInterval:  s.check.config.Schemas.CollectionInterval,
		DbmsVersion:         s.check.dbVersion,
		Tags:                s.check.tags,
		Timestamp:           float64(s.check.clock.Now().UnixMilli()),
		CollectionStartedAt: s.startedAt,
	}
}

func (s *schemaCollector) maybeFlush(isLast bool) {
	if !isLast && s.tableCount < s.payloadChunkSize {
		return
	}
	if s.tableCount == 0 && !isLast {
		return
	}

	s.payloads++
	e := s.baseEvent()
	e.Metadata = []containerObject{{
		ID:      strconv.FormatInt(s.conID, 10),
		Name:    s.conName,
		Schemas: s.schemas,
	}}
	if isLast {
		e.CollectionPayloadsCount = s.payloads
	}
	e.Truncated = s.truncated

	s.emit(e)
	log.Debugf("%s schema payload con_id=%d tables=%d last=%t",
		s.check.logPrompt, s.conID, s.tableCount, isLast)

	s.reset()
}

func (s *schemaCollector) useSchema(conID int64, owner string) {
	if s.currentSchema != nil && s.currentSchema.Name == owner {
		return
	}
	s.currentSchema = &schemaObject{
		ID:    s.owners[ownerKey{conID: conID, owner: owner}],
		Name:  owner,
		Owner: owner,
	}
	s.schemas = append(s.schemas, s.currentSchema)
	s.currentTable = nil
}

func (s *schemaCollector) add(r schemaRowDB) {
	if r.ConID != s.conID {
		s.startContainer(r.ConID)
	}

	// Flush only at table boundaries; the backend cannot merge a table split across payloads.
	newTable := s.currentSchema == nil || s.currentSchema.Name != r.Owner || s.currentTable == nil || s.currentTable.Name != r.TableName
	if newTable {
		s.maybeFlush(false)
	}

	s.useSchema(r.ConID, r.Owner)

	if newTable {
		t := &schemaTable{
			Name:       r.TableName,
			Owner:      r.Owner,
			TableType:  tableType(r),
			Properties: tableProperties(r),
		}
		if r.Temporary == "Y" {
			t.Temporary = &temporaryDetail{Scope: r.Duration}
		}
		if r.ObjectType != "" && r.ObjectType != "-" {
			t.ObjectType = &objectTypeDetail{TypeName: r.ObjectType}
			if r.ObjectTypeOwner != "" && r.ObjectTypeOwner != "-" {
				t.ObjectType.TypeOwner = r.ObjectTypeOwner
			}
		}
		if d := s.details[tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName}]; d != nil {
			t.ID = d.ID
			t.Comment = d.Comment
			for _, idx := range d.Indexes {
				if len(idx.Columns) > 0 {
					t.Indexes = append(t.Indexes, idx)
				}
			}
			t.Constraints = d.Constraints
			t.External = d.External
			if d.Mview != nil {
				t.TableType = "materialized_view"
				t.Mview = d.Mview
			}
			t.Partitioned = d.Partitioned
			t.Blockchain = d.Blockchain
			t.Immutable = d.Immutable
			if d.Blockchain != nil {
				t.Properties = append(t.Properties, "blockchain")
			}
			if d.Immutable != nil {
				t.Properties = append(t.Properties, "immutable")
			}
		}
		s.currentTable = t
		s.currentSchema.Tables = append(s.currentSchema.Tables, s.currentTable)
		s.tableCount++
		s.tablesTotal++

		if r.TotalTables.Valid && r.TotalTables.Int64 > int64(s.check.config.Schemas.MaxTables) {
			s.truncated = true
		}
		if r.TotalColumns.Valid && r.TotalColumns.Int64 > int64(s.check.config.Schemas.MaxColumns) {
			s.truncated = true
		}
	}

	columnPresent := (r.ColumnPresent.Valid && r.ColumnPresent.Int64 == 1) || (!r.ColumnPresent.Valid && r.ColumnName != "")
	if columnPresent {
		col := schemaColumn{
			Name:      r.ColumnName,
			DataType:  dataType(r),
			Nullable:  r.Nullable == "Y",
			Virtual:   r.VirtualColumn == "YES",
			Invisible: r.HiddenColumn == "YES",
		}
		if d := s.details[tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName}]; d != nil {
			col.Comment = d.ColumnComments[r.ColumnName]
			col.Default = d.ColumnDefaults[r.ColumnName]
		}
		s.currentTable.Columns = append(s.currentTable.Columns, col)
	}
}

// Call only after successful collection; the final payload marks the snapshot complete.

func (s *schemaCollector) finish() {
	if s.conID == -1 {
		return
	}
	s.maybeFlush(true)
	s.conID = -1
}

// Empty snapshots clear stale backend metadata when a container produces no rows.
func (s *schemaCollector) emitEmptyContainers(containers map[int64]string) {
	for conID := range containers {
		if _, ok := s.started[conID]; ok {
			continue
		}
		s.startContainer(conID)
		s.finish()
	}
}

func dataType(r schemaRowDB) string {
	if !r.DataType.Valid {
		return ""
	}
	t := r.DataType.String
	var rendered string
	switch t {
	case "NUMBER":
		if r.DataPrecision.Valid {
			scale := int64(0)
			if r.DataScale.Valid {
				scale = r.DataScale.Int64
			}
			rendered = fmt.Sprintf("NUMBER(%d,%d)", r.DataPrecision.Int64, scale)
		} else {
			rendered = t
		}
	case "FLOAT":
		if r.DataPrecision.Valid {
			rendered = fmt.Sprintf("FLOAT(%d)", r.DataPrecision.Int64)
		} else {
			rendered = t
		}
	case "VARCHAR2", "CHAR":
		if r.CharUsed == "C" && r.CharLength.Valid {
			rendered = fmt.Sprintf("%s(%d CHAR)", t, r.CharLength.Int64)
		} else if r.DataLength.Valid {
			rendered = fmt.Sprintf("%s(%d BYTE)", t, r.DataLength.Int64)
		} else {
			rendered = t
		}
	case "NVARCHAR2", "NCHAR":
		// The grammar has no BYTE/CHAR qualifier for national character types.
		if r.CharLength.Valid {
			rendered = fmt.Sprintf("%s(%d)", t, r.CharLength.Int64)
		} else {
			rendered = t
		}
	case "RAW":
		if r.DataLength.Valid {
			rendered = fmt.Sprintf("RAW(%d)", r.DataLength.Int64)
		} else {
			rendered = t
		}
	default:
		// Temporal precision is already in DATA_TYPE; other lengths may be internal locator sizes.
		rendered = t
	}

	if owner := r.DataTypeOwner.String; r.DataTypeOwner.Valid && owner != "" && owner != "SYS" && owner != "PUBLIC" {
		rendered = owner + "." + rendered
	}
	if r.DataTypeMod.Valid && strings.TrimSpace(r.DataTypeMod.String) != "" {
		rendered = strings.TrimSpace(r.DataTypeMod.String) + " " + rendered
	}
	return rendered
}

func tableType(r schemaRowDB) string {
	if r.External == "YES" {
		return "external"
	}
	return "table"
}

func tableProperties(r schemaRowDB) []string {
	var props []string
	if r.Temporary == "Y" {
		props = append(props, "temporary")
	}
	if r.Partitioned == "YES" {
		props = append(props, "partitioned")
	}
	if r.IotType != "-" {
		props = append(props, "index_organized")
	}
	if r.ClusterName != "-" {
		props = append(props, "clustered")
	}
	if r.Clustering == "YES" {
		props = append(props, "attribute_clustered")
	}
	if r.ReadOnly == "YES" {
		props = append(props, "read_only")
	}
	if r.ObjectType != "" && r.ObjectType != "-" {
		props = append(props, "object_table")
	}
	return props
}

// DATA_DEFAULT_VC exists from 23ai. Earlier versions require the LONG DATA_DEFAULT column,
// which cannot be passed to a SQL function and is therefore truncated in Go.

type ownerKey struct {
	conID int64
	owner string
}

func (c *Check) schemaOwners(ctx context.Context, containers map[int64]string) (map[ownerKey]string, []string, error) {
	owners := make(map[ownerKey]string)
	names := make(map[string]struct{})
	query := schemaOwnersQuery + regexSQLClauses("username", c.config.Schemas.IncludeSchemas, c.config.Schemas.ExcludeSchemas)
	err := c.queryMetadata(ctx, query, func(rows *sqlx.Rows) error {
		var (
			conID  int64
			name   string
			userID sql.NullInt64
		)
		if err := rows.Scan(&conID, &name, &userID); err != nil {
			return fmt.Errorf("failed to scan schema owner: %w", err)
		}
		if _, ok := containers[conID]; !ok {
			return nil
		}
		if !schemaOwnerPattern.MatchString(name) {
			log.Warnf("%s skipping schema owner with unexpected characters: %q", c.logPrompt, name)
			return nil
		}
		id := ""
		if userID.Valid {
			id = strconv.FormatInt(userID.Int64, 10)
		}
		owners[ownerKey{conID: conID, owner: name}] = id
		names[name] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query schema owners: %w", err)
	}

	distinct := make([]string, 0, len(names))
	for n := range names {
		distinct = append(distinct, n)
	}
	sort.Strings(distinct)
	return owners, distinct, nil
}

func ownerListChunks(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	chunks := make([]string, 0, (len(names)+maxSchemaOwners-1)/maxSchemaOwners)
	for i := 0; i < len(names); i += maxSchemaOwners {
		end := i + maxSchemaOwners
		if end > len(names) {
			end = len(names)
		}
		batch := names[i:end]
		quoted := make([]string, len(batch))
		for j, n := range batch {
			quoted[j] = "'" + n + "'"
		}
		chunks = append(chunks, strings.Join(quoted, ", "))
	}
	return chunks
}

type relationColumnNames struct {
	conID    string
	owner    string
	relation string
}

type columnKey struct {
	tableKey
	column string
}

func relationFilterChunks(allowed map[tableKey]struct{}, columns relationColumnNames) []string {
	keys := make([]tableKey, 0, len(allowed))
	for key := range allowed {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].conID != keys[j].conID {
			return keys[i].conID < keys[j].conID
		}
		if keys[i].owner != keys[j].owner {
			return keys[i].owner < keys[j].owner
		}
		return keys[i].table < keys[j].table
	})

	filters := make([]string, 0, (len(keys)+maxSchemaRelationsPerQuery-1)/maxSchemaRelationsPerQuery)
	for start := 0; start < len(keys); start += maxSchemaRelationsPerQuery {
		end := start + maxSchemaRelationsPerQuery
		if end > len(keys) {
			end = len(keys)
		}
		var groups []string
		for i := start; i < end; {
			j := i + 1
			for j < end && keys[j].conID == keys[i].conID && keys[j].owner == keys[i].owner {
				j++
			}
			names := make([]string, 0, j-i)
			for _, key := range keys[i:j] {
				names = append(names, "'"+escapeSQLLiteral(key.table)+"'")
			}
			groups = append(groups, fmt.Sprintf("(%s = %d AND %s = '%s' AND %s IN (%s))",
				columns.conID, keys[i].conID,
				columns.owner, escapeSQLLiteral(keys[i].owner),
				columns.relation, strings.Join(names, ", ")))
			i = j
		}
		filters = append(filters, "("+strings.Join(groups, " OR ")+")")
	}
	return filters
}

func columnFilterChunks(allowed map[columnKey]struct{}, columns relationColumnNames, columnName string) []string {
	keys := make([]columnKey, 0, len(allowed))
	for key := range allowed {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].conID != keys[j].conID {
			return keys[i].conID < keys[j].conID
		}
		if keys[i].owner != keys[j].owner {
			return keys[i].owner < keys[j].owner
		}
		if keys[i].table != keys[j].table {
			return keys[i].table < keys[j].table
		}
		return keys[i].column < keys[j].column
	})

	filters := make([]string, 0, (len(keys)+maxSchemaRelationsPerQuery-1)/maxSchemaRelationsPerQuery)
	for start := 0; start < len(keys); start += maxSchemaRelationsPerQuery {
		end := start + maxSchemaRelationsPerQuery
		if end > len(keys) {
			end = len(keys)
		}
		var groups []string
		for i := start; i < end; {
			j := i + 1
			for j < end && keys[j].tableKey == keys[i].tableKey {
				j++
			}
			names := make([]string, 0, j-i)
			for _, key := range keys[i:j] {
				names = append(names, "'"+escapeSQLLiteral(key.column)+"'")
			}
			groups = append(groups, fmt.Sprintf("(%s = %d AND %s = '%s' AND %s = '%s' AND %s IN (%s))",
				columns.conID, keys[i].conID,
				columns.owner, escapeSQLLiteral(keys[i].owner),
				columns.relation, escapeSQLLiteral(keys[i].table),
				columnName, strings.Join(names, ", ")))
			i = j
		}
		filters = append(filters, "("+strings.Join(groups, " OR ")+")")
	}
	return filters
}

func (c *Check) queryMetadata(ctx context.Context, query string, scan func(*sqlx.Rows) error) error {
	queryCtx, cancel := context.WithTimeout(ctx, c.config.Schemas.MaxQueryDurationDuration())
	defer cancel()

	rows, err := c.db.QueryxContext(queryCtx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Missing or version-incompatible optional detail views do not fail collection.

func (c *Check) defaultValueColumn() string {
	major, _, _ := strings.Cut(c.dbVersion, ".")
	if n, err := strconv.Atoi(major); err == nil && n >= 23 {
		return "c.data_default_vc"
	}
	return "c.data_default"
}

// SEARCH_CONDITION_VC exists from 12c; earlier versions require LONG SEARCH_CONDITION.
func (c *Check) conditionColumn() string {
	major, _, _ := strings.Cut(c.dbVersion, ".")
	if n, err := strconv.Atoi(major); err == nil && n >= 12 {
		return "c.search_condition_vc"
	}
	return "c.search_condition"
}

// Function-based index expressions use the same 23ai _VC cutover under the tc alias.
func (c *Check) indexExpressionColumn() string {
	major, _, _ := strings.Cut(c.dbVersion, ".")
	if n, err := strconv.Atoi(major); err == nil && n >= 23 {
		return "tc.data_default_vc"
	}
	return "tc.data_default"
}

// Cap fallback LONG values at 4000 characters without splitting multi-byte characters.
func truncateLongValue(s string) string {
	r := []rune(s)
	if len(r) <= 4000 {
		return s
	}
	return string(r[:4000])
}

func (c *Check) queryDetailFilters(ctx context.Context, name, template string, filters []string, scan func(*sqlx.Rows) error) {
	for _, filter := range filters {
		query := strings.Replace(template, "/*RELATIONS*/", filter, 1)
		if err := c.queryMetadata(ctx, query, scan); err != nil {
			if strings.Contains(err.Error(), oracleErrorTableOrViewDoesNotExist) || strings.Contains(err.Error(), oracleErrorInvalidIdentifier) {
				log.Debugf("%s table detail %q unavailable: %s", c.logPrompt, name, err)
				return
			}
			log.Warnf("%s failed to collect table detail %q: %s", c.logPrompt, name, err)
			return
		}
	}
}

func (c *Check) queryDetails(ctx context.Context, name, template string, allowed map[tableKey]struct{}, columns relationColumnNames, scan func(*sqlx.Rows) error) {
	c.queryDetailFilters(ctx, name, template, relationFilterChunks(allowed, columns), scan)
}

func constraintType(t string) string {
	switch t {
	case "P":
		return "primary_key"
	case "U":
		return "unique"
	case "R":
		return "foreign_key"
	case "C":
		return "check"
	default:
		return t
	}
}

func (c *Check) validateSchemaContainer(ctx context.Context, containerID string) error {
	conID, err := strconv.ParseInt(containerID, 10, 64)
	if err != nil {
		return err
	}
	available := false
	query := containerNamesQuery + fmt.Sprintf(" AND con_id = %d", conID)
	if err := c.queryMetadata(ctx, query, func(rows *sqlx.Rows) error {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		available = true
		return nil
	}); err != nil {
		return fmt.Errorf("failed to validate container %d: %w", conID, err)
	}
	if !available {
		return fmt.Errorf("container %d is no longer available for schema collection", conID)
	}
	return nil
}

func (c *Check) containerNames(ctx context.Context) (map[int64]string, error) {
	names := make(map[int64]string)
	query := containerNamesQuery + regexSQLClauses("name", c.config.Schemas.IncludeDatabases, c.config.Schemas.ExcludeDatabases)
	err := c.queryMetadata(ctx, query, func(rows *sqlx.Rows) error {
		var (
			conID int64
			name  string
		)
		if err := rows.Scan(&conID, &name); err != nil {
			return fmt.Errorf("failed to scan container name: %w", err)
		}
		names[conID] = name
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query container names: %w", err)
	}
	return names, nil
}

func (c *Check) tableDetails(ctx context.Context, allowed map[tableKey]struct{}, allowedColumns map[columnKey]struct{}) map[tableKey]*tableDetails {
	return c.tableDetailsForPage(ctx, allowed, allowedColumns, allowed)
}

func (c *Check) tableDetailsForPage(ctx context.Context, allowed map[tableKey]struct{}, allowedColumns map[columnKey]struct{}, selectedTables map[tableKey]struct{}) map[tableKey]*tableDetails {
	details := make(map[tableKey]*tableDetails)
	at := func(conID int64, owner, table string) *tableDetails {
		k := tableKey{conID: conID, owner: owner, table: table}
		if _, ok := allowed[k]; !ok {
			return &tableDetails{}
		}
		if details[k] == nil {
			details[k] = &tableDetails{}
		}
		return details[k]
	}

	scanRetention := func(withHash bool) func(*sqlx.Rows) error {
		return func(rows *sqlx.Rows) error {
			var (
				conID                int64
				owner, table         string
				retention, inactive  sql.NullInt64
				locked               string
				hashAlgo, tabVersion sql.NullString
			)
			var err error
			if withHash {
				err = rows.Scan(&conID, &owner, &table, &retention, &locked, &inactive, &hashAlgo, &tabVersion)
			} else {
				err = rows.Scan(&conID, &owner, &table, &retention, &locked, &inactive)
			}
			if err != nil {
				return err
			}
			d := &retentionDetail{RowRetentionLocked: locked == "YES"}
			if retention.Valid {
				d.RowRetentionDays = &retention.Int64
			}
			if inactive.Valid {
				d.InactivityRetentionDays = &inactive.Int64
			}
			if hashAlgo.Valid {
				d.HashAlgorithm = hashAlgo.String
			}
			if tabVersion.Valid {
				d.TableVersion = tabVersion.String
			}
			if withHash {
				at(conID, owner, table).Blockchain = d
			} else {
				at(conID, owner, table).Immutable = d
			}
			return nil
		}
	}

	c.queryDetails(ctx, "blockchain", blockchainTablesQuery, allowed, relationColumnNames{conID: "con_id", owner: "schema_name", relation: "table_name"}, scanRetention(true))
	c.queryDetails(ctx, "immutable", immutableTablesQuery, allowed, relationColumnNames{conID: "con_id", owner: "schema_name", relation: "table_name"}, scanRetention(false))

	c.queryDetails(ctx, "materialized views", mviewsQuery, allowed, relationColumnNames{conID: "con_id", owner: "owner", relation: "mview_name"}, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, name, mode, method, staleness string
		var lastRefresh sql.NullTime
		if err := rows.Scan(&conID, &owner, &name, &mode, &method, &staleness, &lastRefresh); err != nil {
			return err
		}
		d := &mviewDetail{}
		if mode != "-" {
			d.RefreshMode = mode
		}
		if method != "-" {
			d.RefreshMethod = method
		}
		if staleness != "-" {
			d.Staleness = staleness
		}
		if lastRefresh.Valid {
			d.LastRefreshDate = lastRefresh.Time.UTC().Format(time.RFC3339)
		}
		at(conID, owner, name).Mview = d
		return nil
	})

	c.queryDetails(ctx, "table comments", tableCommentsQuery, allowed, relationColumnNames{conID: "con_id", owner: "owner", relation: "table_name"}, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, comment string
		if err := rows.Scan(&conID, &owner, &table, &comment); err != nil {
			return err
		}
		at(conID, owner, table).Comment = comment
		return nil
	})

	commentFilters := columnFilterChunks(allowedColumns,
		relationColumnNames{conID: "con_id", owner: "owner", relation: "table_name"}, "column_name")
	c.queryDetailFilters(ctx, "column comments", columnCommentsQuery, commentFilters, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, column, comment string
		if err := rows.Scan(&conID, &owner, &table, &column, &comment); err != nil {
			return err
		}
		d := at(conID, owner, table)
		if d.ColumnComments == nil {
			d.ColumnComments = make(map[string]string)
		}
		d.ColumnComments[column] = comment
		return nil
	})

	defaultsQueryResolved := strings.Replace(columnDefaultsQuery, "/*DEFAULT_COL*/", c.defaultValueColumn(), 1)
	defaultFilters := columnFilterChunks(allowedColumns,
		relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.table_name"}, "c.column_name")
	c.queryDetailFilters(ctx, "column defaults", defaultsQueryResolved, defaultFilters, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, column string
		var value sql.NullString
		if err := rows.Scan(&conID, &owner, &table, &column, &value); err != nil {
			return err
		}
		if value.Valid {
			d := at(conID, owner, table)
			if d.ColumnDefaults == nil {
				d.ColumnDefaults = make(map[string]string)
			}
			d.ColumnDefaults[column] = truncateLongValue(value.String)
		}
		return nil
	})

	indexesQueryResolved := strings.Replace(indexesQuery, "/*EXPRESSION_COL*/", c.indexExpressionColumn(), 1)
	c.queryDetails(ctx, "indexes", indexesQueryResolved, allowed, relationColumnNames{conID: "i.con_id", owner: "i.table_owner", relation: "i.table_name"}, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, indexOwner, name, uniqueness, indexType, column string
		var expression sql.NullString
		if err := rows.Scan(&conID, &owner, &table, &indexOwner, &name, &uniqueness, &indexType, &column, &expression); err != nil {
			return err
		}
		d := at(conID, owner, table)
		var idx *indexInfo
		if n := len(d.Indexes); n > 0 && d.Indexes[n-1].owner == indexOwner && d.Indexes[n-1].Name == name {
			idx = d.Indexes[n-1]
		} else {
			idx = &indexInfo{owner: indexOwner, Name: name, Unique: uniqueness == "UNIQUE", Type: indexType}
			d.Indexes = append(d.Indexes, idx)
		}
		if expression.Valid {
			idx.Columns = append(idx.Columns, indexKeyPart{Expression: truncateLongValue(expression.String)})
		} else {
			idx.Columns = append(idx.Columns, indexKeyPart{Column: column})
		}
		return nil
	})

	// Foreign keys identify referenced constraints, so resolve their tables after scanning all rows.
	primaryKeys := make(map[constraintKey]*constraintInfo)
	constraintsQueryResolved := strings.Replace(constraintsQuery, "/*CONDITION_COL*/", c.conditionColumn(), 1)
	c.queryDetails(ctx, "constraints", constraintsQueryResolved, allowed, relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.table_name"}, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, name, ctype, rOwner, rName, column string
		var condition sql.NullString
		if err := rows.Scan(&conID, &owner, &table, &name, &ctype, &rOwner, &rName, &column, &condition); err != nil {
			return err
		}
		d := at(conID, owner, table)
		var con *constraintInfo
		if n := len(d.Constraints); n > 0 && d.Constraints[n-1].Name == name {
			con = d.Constraints[n-1]
		} else {
			con = &constraintInfo{Name: name, Type: constraintType(ctype)}
			if ctype == "C" && condition.Valid {
				con.Condition = truncateLongValue(condition.String)
			}
			if ctype == "R" {
				con.ReferencedOwner = rOwner
				con.referencedKey = &constraintKey{conID: conID, owner: rOwner, name: rName}
				if rName != "-" {
					con.referencedName = rName
				}
			}
			d.Constraints = append(d.Constraints, con)
		}
		con.Columns = append(con.Columns, column)
		if ctype == "P" || ctype == "U" {
			primaryKeys[constraintKey{conID: conID, owner: owner, name: name}] = &constraintInfo{
				ReferencedTable:   table,
				ReferencedColumns: con.Columns,
			}
		}
		return nil
	})
	for _, d := range details {
		for _, con := range d.Constraints {
			if con.referencedKey == nil {
				continue
			}
			if target, ok := primaryKeys[*con.referencedKey]; ok {
				con.ReferencedTable = target.ReferencedTable
				con.ReferencedColumns = target.ReferencedColumns
			} else {
				// Preserve the constraint name when its owner was not collected.
				con.ReferencedConstraint = con.referencedName
			}
		}
	}
	referenced := make(map[constraintKey][]*constraintInfo)
	for _, d := range details {
		for _, con := range d.Constraints {
			if con.referencedKey != nil && con.ReferencedTable == "" {
				referenced[*con.referencedKey] = append(referenced[*con.referencedKey], con)
			}
		}
	}
	referencedKeys := make(map[tableKey]struct{}, len(referenced))
	for key := range referenced {
		referencedKeys[tableKey{conID: key.conID, owner: key.owner, table: key.name}] = struct{}{}
	}
	filters := relationFilterChunks(referencedKeys,
		relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.constraint_name"})
	c.queryDetailFilters(ctx, "referenced constraints", referencedConstraintsQuery, filters, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, name, table, column string
		if err := rows.Scan(&conID, &owner, &name, &table, &column); err != nil {
			return err
		}
		constraints := referenced[constraintKey{conID: conID, owner: owner, name: name}]
		if len(constraints) == 0 {
			return nil
		}
		if _, ok := selectedTables[tableKey{conID: conID, owner: owner, table: table}]; !ok {
			return nil
		}
		for _, con := range constraints {
			con.ReferencedTable = table
			con.ReferencedColumns = append(con.ReferencedColumns, column)
			con.ReferencedConstraint = ""
		}
		return nil
	})

	c.queryDetails(ctx, "external tables", externalTablesQuery, allowed, relationColumnNames{conID: "et.con_id", owner: "et.owner", relation: "et.table_name"}, func(rows *sqlx.Rows) error {
		var (
			conID                  int64
			owner, table, driver   string
			directory, locationDir string
			location               sql.NullString
		)
		if err := rows.Scan(&conID, &owner, &table, &driver, &directory, &locationDir, &location); err != nil {
			return err
		}
		d := at(conID, owner, table)
		if d.External == nil {
			d.External = &externalDetail{}
		}
		d.External.AccessDriver = driver
		if directory != "-" {
			d.External.Directory = directory
		}
		if location.Valid {
			loc := location.String
			if locationDir != "-" {
				loc = locationDir + ":" + loc
			}
			d.External.Locations = append(d.External.Locations, loc)
		}
		return nil
	})

	c.queryDetails(ctx, "object ids", objectIDsQuery, allowed, relationColumnNames{conID: "con_id", owner: "owner", relation: "object_name"}, func(rows *sqlx.Rows) error {
		var (
			conID        int64
			owner, table string
			objectID     sql.NullInt64
		)
		if err := rows.Scan(&conID, &owner, &table, &objectID); err != nil {
			return err
		}
		if objectID.Valid {
			at(conID, owner, table).ID = strconv.FormatInt(objectID.Int64, 10)
		}
		return nil
	})

	keys := make(map[tableKey][]string)
	c.queryDetails(ctx, "partitioning", partTablesQuery, allowed, relationColumnNames{conID: "pt.con_id", owner: "pt.owner", relation: "pt.table_name"}, func(rows *sqlx.Rows) error {
		var (
			conID          int64
			owner, table   string
			ptype, subtype sql.NullString
			count          sql.NullInt64
			col            sql.NullString
		)
		if err := rows.Scan(&conID, &owner, &table, &ptype, &subtype, &count, &col); err != nil {
			return err
		}
		d := at(conID, owner, table)
		if d.Partitioned == nil {
			d.Partitioned = &partitionDetail{PartitioningType: ptype.String, NumPartitions: count.Int64}
			if subtype.Valid && subtype.String != "NONE" {
				d.Partitioned.SubpartitionsType = subtype.String
			}
		}
		if col.Valid {
			k := tableKey{conID: conID, owner: owner, table: table}
			keys[k] = append(keys[k], col.String)
		}
		return nil
	})
	for k, cols := range keys {
		if d := details[k]; d != nil && d.Partitioned != nil {
			d.Partitioned.PartitionKey = fmt.Sprintf("%s (%s)", d.Partitioned.PartitioningType, strings.Join(cols, ", "))
		}
	}

	return details
}

func (c *Check) tableIdentities(ctx context.Context, ownerLists []string, owners map[ownerKey]string, tableFilters string, maxTables int) ([]tableKey, map[int64]struct{}, error) {
	byContainer := make(map[int64][]tableKey)
	seen := make(map[tableKey]struct{})
	containerSet := make(map[int64]struct{})
	for key := range owners {
		containerSet[key.conID] = struct{}{}
	}
	containerIDs := make([]int64, 0, len(containerSet))
	for conID := range containerSet {
		containerIDs = append(containerIDs, conID)
	}
	sort.Slice(containerIDs, func(i, j int) bool { return containerIDs[i] < containerIDs[j] })
	for _, conID := range containerIDs {
		for _, ownerList := range ownerLists {
			remaining := maxTables + 1 - len(byContainer[conID])
			if remaining == 0 {
				break
			}
			query := strings.ReplaceAll(tableIdentitiesQueryTemplate, "/*OWNERS*/", ownerList)
			query = strings.ReplaceAll(query, "/*TABLE_FILTERS*/", tableFilters)
			query = strings.ReplaceAll(query, "/*CON_ID*/", strconv.FormatInt(conID, 10))
			query = strings.ReplaceAll(query, "/*IDENTITY_LIMIT*/", strconv.Itoa(remaining))
			err := c.queryMetadata(ctx, query, func(rows *sqlx.Rows) error {
				var (
					rowConID int64
					owner    string
					table    string
				)
				if err := rows.Scan(&rowConID, &owner, &table); err != nil {
					return err
				}
				if _, ok := owners[ownerKey{conID: rowConID, owner: owner}]; !ok {
					return nil
				}
				key := tableKey{conID: rowConID, owner: owner, table: table}
				if _, ok := seen[key]; ok {
					return nil
				}
				seen[key] = struct{}{}
				byContainer[rowConID] = append(byContainer[rowConID], key)
				return nil
			})
			if err != nil {
				return nil, nil, err
			}
		}
	}

	truncated := make(map[int64]struct{})
	var keys []tableKey
	for conID, containerKeys := range byContainer {
		if len(containerKeys) > maxTables {
			truncated[conID] = struct{}{}
			containerKeys = containerKeys[:maxTables]
		}
		keys = append(keys, containerKeys...)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].conID != keys[j].conID {
			return keys[i].conID < keys[j].conID
		}
		if keys[i].owner != keys[j].owner {
			return keys[i].owner < keys[j].owner
		}
		return keys[i].table < keys[j].table
	})
	return keys, truncated, nil
}

func (c *Check) hydrateTablePage(ctx context.Context, keys []tableKey, maxColumns int, add func(schemaRowDB)) error {
	rows, err := c.tablePageRows(ctx, keys, maxColumns)
	if err != nil {
		return err
	}
	for _, row := range rows {
		add(row)
	}
	return nil
}

func (c *Check) tablePageRows(ctx context.Context, keys []tableKey, maxColumns int) ([]schemaRowDB, error) {
	allowed := make(map[tableKey]struct{}, len(keys))
	var pageRows []schemaRowDB
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	filters := relationFilterChunks(allowed, relationColumnNames{conID: "t.con_id", owner: "t.owner", relation: "t.table_name"})
	columnFilters := relationFilterChunks(allowed, relationColumnNames{conID: "c.con_id", owner: "c.owner", relation: "c.table_name"})
	for i, filter := range filters {
		query := strings.ReplaceAll(schemasQueryTemplate, "/*OWNERS*/", ownerListForKeys(keys))
		query = strings.ReplaceAll(query, "/*RELATIONS*/", filter)
		query = strings.ReplaceAll(query, "/*COLUMN_RELATIONS*/", columnFilters[i])
		query = strings.ReplaceAll(query, "/*MAX_COLUMNS*/", strconv.Itoa(maxColumns))
		if err := c.queryMetadata(ctx, query, func(rows *sqlx.Rows) error {
			var row schemaRowDB
			if err := rows.StructScan(&row); err != nil {
				return err
			}
			pageRows = append(pageRows, row)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return pageRows, nil
}

func ownerListForKeys(keys []tableKey) string {
	owners := make(map[string]struct{})
	for _, key := range keys {
		owners[key.owner] = struct{}{}
	}
	names := make([]string, 0, len(owners))
	for owner := range owners {
		names = append(names, owner)
	}
	sort.Strings(names)
	for i := range names {
		names[i] = "'" + escapeSQLLiteral(names[i]) + "'"
	}
	return strings.Join(names, ", ")
}

func forEachTablePage(keys []tableKey, process func([]tableKey) error) error {
	for start := 0; start < len(keys); {
		end := start + schemaRelationPageSize
		if end > len(keys) {
			end = len(keys)
		}
		for i := start + 1; i < end; i++ {
			if keys[i].conID != keys[start].conID {
				end = i
				break
			}
		}
		if err := process(keys[start:end]); err != nil {
			return err
		}
		start = end
	}
	return nil
}

func tableKeysFromRows(rows []schemaRowDB) map[tableKey]struct{} {
	keys := make(map[tableKey]struct{}, len(rows))
	for _, r := range rows {
		keys[tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName}] = struct{}{}
	}
	return keys
}

func columnKeysFromRows(rows []schemaRowDB) map[columnKey]struct{} {
	keys := make(map[columnKey]struct{}, len(rows))
	for _, r := range rows {
		keys[columnKey{
			tableKey: tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName},
			column:   r.ColumnName,
		}] = struct{}{}
	}
	return keys
}

func schemaCollectionVersionSupported(version string) bool {
	major, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(major)
	return err == nil && n >= 12
}

func (c *Check) SchemaCollection() error {
	return c.schemaCollection(context.Background())
}

func (c *Check) schemaCollection(ctx context.Context) error {
	if !schemaCollectionVersionSupported(c.dbVersion) {
		log.Warnf("%s schema collection requires Oracle %sc or later", c.logPrompt, minMultitenantVersion)
		return nil
	}

	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}

	containers, err := c.containerNames(ctx)
	if err != nil {
		return err
	}

	owners, names, err := c.schemaOwners(ctx, containers)
	if err != nil {
		return err
	}

	emit := func(payload []byte) {
		sender.EventPlatformEvent(payload, "dbm-metadata")
	}
	coordinator := newSchemaSnapshotCoordinator(emit)
	coordinator.validate = func(containerID string) error {
		return c.validateSchemaContainer(ctx, containerID)
	}

	if len(names) == 0 {
		log.Debugf("%s no user schemas to collect, sending empty snapshot", c.logPrompt)
		newSchemaEventCollector(c, coordinator.add, nil, owners, containers).emitEmptyContainers(containers)
		if err := coordinator.complete(); err != nil {
			return err
		}
		sender.Commit()
		return nil
	}

	tableFilters := regexSQLClauses("t.table_name", c.config.Schemas.IncludeTables, c.config.Schemas.ExcludeTables)
	keys, cappedContainers, err := c.tableIdentities(ctx, ownerListChunks(names), owners, tableFilters, c.config.Schemas.MaxTables)
	if err != nil {
		return fmt.Errorf("failed to query table identities: %w", err)
	}

	tablesTotal := 0
	activeContainer := int64(-1)
	selectedTables := make(map[tableKey]struct{}, len(keys))
	for _, key := range keys {
		selectedTables[key] = struct{}{}
	}
	if err := forEachTablePage(keys, func(page []tableKey) error {
		conID := page[0].conID
		if activeContainer != -1 && conID != activeContainer {
			if err := coordinator.completeContainer(activeContainer); err != nil {
				return err
			}
		}
		activeContainer = conID
		rows, err := c.tablePageRows(ctx, page, c.config.Schemas.MaxColumns)
		if err != nil {
			return err
		}
		pageTables := tableKeysFromRows(rows)
		details := c.tableDetailsForPage(ctx, pageTables, columnKeysFromRows(rows), selectedTables)
		collector := newSchemaEventCollector(c, coordinator.add, details, owners, containers)
		collector.truncatedContainers = cappedContainers
		for _, row := range rows {
			collector.add(row)
		}
		collector.finish()
		tablesTotal += collector.tablesTotal
		return coordinator.err
	}); err != nil {
		sender.Commit()
		return fmt.Errorf("failed to query schemas: %w", err)
	}
	if activeContainer != -1 {
		if err := coordinator.completeContainer(activeContainer); err != nil {
			return err
		}
	}
	emptyContainers := make(map[int64]string)
	for conID, name := range containers {
		if !coordinator.hasContainer(conID) {
			emptyContainers[conID] = name
		}
	}
	emptyContainerIDs := make([]int64, 0, len(emptyContainers))
	for conID := range emptyContainers {
		emptyContainerIDs = append(emptyContainerIDs, conID)
	}
	sort.Slice(emptyContainerIDs, func(i, j int) bool { return emptyContainerIDs[i] < emptyContainerIDs[j] })
	for _, conID := range emptyContainerIDs {
		container := map[int64]string{conID: emptyContainers[conID]}
		collector := newSchemaEventCollector(c, coordinator.add, nil, owners, container)
		collector.truncatedContainers = cappedContainers
		collector.emitEmptyContainers(container)
		if err := coordinator.completeContainer(conID); err != nil {
			return err
		}
	}

	for conID := range cappedContainers {
		log.Warnf("%s table collection stopped at max_tables=%d for container %d; some tables were not collected",
			c.logPrompt, c.config.Schemas.MaxTables, conID)
	}
	log.Debugf("%s schema collection sent %d tables", c.logPrompt, tablesTotal)
	sender.Commit()
	return nil
}
