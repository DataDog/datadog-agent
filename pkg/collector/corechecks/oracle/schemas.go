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
	t.num_rows,
	t.last_analyzed,
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
	t.num_rows,
	t.last_analyzed,
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

const tabModificationsQuery = `SELECT con_id, table_owner, table_name, inserts, updates, deletes, truncated, timestamp
FROM cdb_tab_modifications WHERE partition_name IS NULL AND /*RELATIONS*/`

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
const indexesQuery = `SELECT i.con_id, i.table_owner, i.table_name, i.index_name, i.uniqueness, i.index_type,
	ic.column_name, /*EXPRESSION_COL*/ AS column_expression
FROM cdb_indexes i
JOIN cdb_ind_columns ic
	ON ic.con_id = i.con_id AND ic.index_owner = i.owner AND ic.index_name = i.index_name
LEFT JOIN cdb_tab_cols tc
	ON tc.con_id = ic.con_id AND tc.owner = ic.table_owner AND tc.table_name = ic.table_name
	AND tc.column_name = ic.column_name AND ic.column_name LIKE 'SYS\_NC%' ESCAPE '\'
WHERE /*RELATIONS*/
ORDER BY i.con_id, i.table_owner, i.table_name, i.index_name, ic.column_position`

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

const containerNamesQuery = `SELECT con_id, name FROM v$containers`

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
	CAST(NULL AS NUMBER) AS num_rows,
	CAST(NULL AS DATE) AS last_analyzed,
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

func compiledPatterns(patterns []string, logPrompt, kind string) []*regexp.Regexp {
	if len(patterns) == 0 {
		return nil
	}
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			log.Warnf("%s invalid %s pattern %q: %s", logPrompt, kind, p, err)
			continue
		}
		compiled = append(compiled, re)
	}
	return compiled
}

func matchesAny(name string, patterns []*regexp.Regexp) bool {
	for _, re := range patterns {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

func passesFilter(name string, include, exclude []*regexp.Regexp) bool {
	if matchesAny(name, exclude) {
		return false
	}
	return len(include) == 0 || matchesAny(name, include)
}

func escapeSQLLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func regexSQLClauses(column string, include, exclude []string) string {
	var b strings.Builder
	for _, p := range exclude {
		b.WriteString(" AND NOT REGEXP_LIKE(" + column + ", '" + escapeSQLLiteral(p) + "')")
	}
	if len(include) > 0 {
		parts := make([]string, len(include))
		for i, p := range include {
			parts[i] = "REGEXP_LIKE(" + column + ", '" + escapeSQLLiteral(p) + "')"
		}
		b.WriteString(" AND (" + strings.Join(parts, " OR ") + ")")
	}
	return b.String()
}

// Oracle database filters match CDB root and PDB names.
func filterContainers(containers map[int64]string, include, exclude []string, logPrompt string) map[int64]string {
	if len(include) == 0 && len(exclude) == 0 {
		return containers
	}
	includeRe := compiledPatterns(include, logPrompt, "include_databases")
	excludeRe := compiledPatterns(exclude, logPrompt, "exclude_databases")
	filtered := make(map[int64]string, len(containers))
	for conID, name := range containers {
		if passesFilter(name, includeRe, excludeRe) {
			filtered[conID] = name
		}
	}
	return filtered
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
	NumRows          sql.NullInt64  `db:"NUM_ROWS"`
	LastAnalyzed     sql.NullTime   `db:"LAST_ANALYZED"`
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

type modificationsDetail struct {
	Inserts      int64  `json:"inserts"`
	Updates      int64  `json:"updates"`
	Deletes      int64  `json:"deletes"`
	Truncated    bool   `json:"truncated"`
	LastModified string `json:"last_modified,omitempty"`
}

type indexKeyPart struct {
	Column     string `json:"column,omitempty"`
	Expression string `json:"expression,omitempty"`
}

type indexInfo struct {
	Name    string         `json:"name"`
	Unique  bool           `json:"unique"`
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
	referencedKey        string
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
	Modifications  *modificationsDetail
	Temporary      *temporaryDetail
	Partitioned    *partitionDetail
	Blockchain     *retentionDetail
	Immutable      *retentionDetail
}

type schemaTable struct {
	ID            string               `json:"id,omitempty"`
	Name          string               `json:"name"`
	Owner         string               `json:"owner"`
	TableType     string               `json:"table_type"`
	Properties    []string             `json:"table_properties,omitempty"`
	Temporary     *temporaryDetail     `json:"temporary_details,omitempty"`
	Partitioned   *partitionDetail     `json:"partitioned_details,omitempty"`
	Blockchain    *retentionDetail     `json:"blockchain_details,omitempty"`
	Immutable     *retentionDetail     `json:"immutable_details,omitempty"`
	Modifications *modificationsDetail `json:"modifications_details,omitempty"`
	ObjectType    *objectTypeDetail    `json:"object_type_details,omitempty"`
	RowCount      *int64               `json:"row_count_estimate,omitempty"`
	NumRows       *int64               `json:"num_rows,omitempty"`
	LastAnalyzed  string               `json:"last_analyzed,omitempty"`
	Comment       string               `json:"comment,omitempty"`
	External      *externalDetail      `json:"external_details,omitempty"`
	Mview         *mviewDetail         `json:"materialized_view_details,omitempty"`
	Indexes       []*indexInfo         `json:"indexes,omitempty"`
	Constraints   []*constraintInfo    `json:"constraints,omitempty"`
	Columns       []schemaColumn       `json:"columns"`
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
	for _, containerID := range c.order {
		c.completeContainerID(containerID)
	}
	return c.err
}

func (c *schemaSnapshotCoordinator) completeContainer(conID int64) error {
	c.completeContainerID(strconv.FormatInt(conID, 10))
	return c.err
}

func (c *schemaSnapshotCoordinator) completeContainerID(containerID string) {
	snapshot := c.snapshots[containerID]
	if snapshot == nil || snapshot.pending == nil {
		return
	}
	snapshot.pending.CollectionPayloadsCount = snapshot.count
	c.emitEvent(*snapshot.pending)
	snapshot.pending = nil
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
		if r.NumRows.Valid {
			n := r.NumRows.Int64
			t.NumRows = &n
			t.RowCount = &n
		}
		if r.LastAnalyzed.Valid {
			t.LastAnalyzed = r.LastAnalyzed.Time.UTC().Format(time.RFC3339)
		}
		if r.ObjectType != "" && r.ObjectType != "-" {
			t.ObjectType = &objectTypeDetail{TypeName: r.ObjectType}
			if r.ObjectTypeOwner != "" && r.ObjectTypeOwner != "-" {
				t.ObjectType.TypeOwner = r.ObjectTypeOwner
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
		s.currentTable.Columns = append(s.currentTable.Columns, schemaColumn{
			Name:      r.ColumnName,
			DataType:  dataType(r),
			Nullable:  r.Nullable == "Y",
			Virtual:   r.VirtualColumn == "YES",
			Invisible: r.HiddenColumn == "YES",
		})
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
	include := compiledPatterns(c.config.Schemas.IncludeSchemas, c.logPrompt, "include_schemas")
	exclude := compiledPatterns(c.config.Schemas.ExcludeSchemas, c.logPrompt, "exclude_schemas")
	// A failed or stale container lookup must not drop schemas unless database filters require it.
	filterDatabases := len(c.config.Schemas.IncludeDatabases) > 0 || len(c.config.Schemas.ExcludeDatabases) > 0

	owners := make(map[ownerKey]string)
	names := make(map[string]struct{})
	err := c.queryMetadata(ctx, schemaOwnersQuery, func(rows *sqlx.Rows) error {
		var (
			conID  int64
			name   string
			userID sql.NullInt64
		)
		if err := rows.Scan(&conID, &name, &userID); err != nil {
			return fmt.Errorf("failed to scan schema owner: %w", err)
		}
		if filterDatabases {
			if _, ok := containers[conID]; !ok {
				return nil
			}
		}
		if !schemaOwnerPattern.MatchString(name) {
			log.Warnf("%s skipping schema owner with unexpected characters: %q", c.logPrompt, name)
			return nil
		}
		if !passesFilter(name, include, exclude) {
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

func (c *Check) containerNames(ctx context.Context) map[int64]string {
	names := make(map[int64]string)
	err := c.queryMetadata(ctx, containerNamesQuery, func(rows *sqlx.Rows) error {
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
		log.Warnf("%s failed to query container names: %s", c.logPrompt, err)
	}
	return names
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
	allowed := make(map[tableKey]struct{}, len(keys))
	hydrated := make(map[tableKey]struct{}, len(keys))
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
			hydrated[tableKey{conID: row.ConID, owner: row.Owner, table: row.TableName}] = struct{}{}
			add(row)
			return nil
		}); err != nil {
			return err
		}
	}
	for key := range allowed {
		if _, ok := hydrated[key]; !ok {
			return fmt.Errorf("selected table %d.%s.%s disappeared before hydration", key.conID, key.owner, key.table)
		}
	}
	return nil
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

	containers := filterContainers(c.containerNames(ctx), c.config.Schemas.IncludeDatabases, c.config.Schemas.ExcludeDatabases, c.logPrompt)
	owners, names, err := c.schemaOwners(ctx, containers)
	if err != nil {
		return err
	}

	emit := func(payload []byte) {
		sender.EventPlatformEvent(payload, "dbm-metadata")
	}
	coordinator := newSchemaSnapshotCoordinator(emit)

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
	if err := forEachTablePage(keys, func(page []tableKey) error {
		conID := page[0].conID
		if activeContainer != -1 && conID != activeContainer {
			if err := coordinator.completeContainer(activeContainer); err != nil {
				return err
			}
		}
		activeContainer = conID
		collector := newSchemaEventCollector(c, coordinator.add, nil, owners, containers)
		collector.truncatedContainers = cappedContainers
		if err := c.hydrateTablePage(ctx, page, c.config.Schemas.MaxColumns, collector.add); err != nil {
			return err
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
		newSchemaEventCollector(c, coordinator.add, nil, owners, container).emitEmptyContainers(container)
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
