// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

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

// Every payload of a snapshot carries the same collection_started_at, and the final one
// carries collection_payloads_count, so the backend knows when a snapshot is complete. The
// snapshot unit is one container. Same contract as the Python SchemaCollector.

const schemaOwnersQuery = `SELECT con_id, username, user_id FROM cdb_users WHERE oracle_maintained = 'N'`

// object_id is only unique within a container, so it is always paired with con_id.
const objectIDsQuery = `SELECT con_id, owner, object_name, object_id FROM cdb_objects
WHERE object_type = 'TABLE' AND owner IN (/*OWNERS*/)`

// CDB_* views are CONTAINERS()-based, so they must never be scanned wholesale: without the
// owner predicate below this statement ran 336s for 42M buffer gets on a 4-PDB instance
// (0 rows returned), and joining cdb_users in as a third CDB view was no better. Constrained
// to the owners from schemaOwnersQuery it returns in well under a second.
//
// Object tables (CREATE TABLE t OF some_type) never appear in cdb_tables -- only in
// cdb_object_tables -- so they need a second branch. Their columns are still exposed through
// cdb_tab_cols like any other table, so only the table-shape half of the query changes.
// cdb_object_tables has no CLUSTERING or READ_ONLY column (an object table can be neither),
// so those two are hardcoded 'NO' on that branch instead of selected as ORA-00904.
//
// max_tables has to be enforced on ranked_tables, a table-level subquery, and not on the joined
// rows below it: Oracle has no LIMIT, and ROW_NUMBER()/FETCH FIRST applied after the column join
// would cut a table off partway through its columns instead of dropping whole tables. The window
// total (rt.total_tables) rides along on every row so the collector can tell a capped container
// from one that legitimately has fewer tables than max_tables, without a second round trip.
//
// max_columns is enforced the same way, one level down: ranked_columns ranks cdb_tab_cols by
// internal_column_id within each table so the retained columns are the first N in column order,
// and total_columns rides along per row so the collector can tell a capped table from one that
// legitimately has fewer columns than max_columns.
const schemasQueryTemplate = `WITH ranked_tables AS (
	SELECT con_id, owner, table_name, is_object,
		ROW_NUMBER() OVER (PARTITION BY con_id ORDER BY owner, table_name) AS rn,
		COUNT(*) OVER (PARTITION BY con_id) AS total_tables
	FROM (
		SELECT t.con_id, t.owner, t.table_name, 'N' AS is_object
		FROM cdb_tables t
		WHERE t.nested = 'NO'
			AND t.secondary = 'N'
			AND NVL(t.dropped, 'NO') = 'NO'
			AND (t.iot_type IS NULL OR t.iot_type = 'IOT')
			AND t.table_name NOT LIKE 'BIN$%'
			AND t.owner IN (/*OWNERS*/)
			/*TABLE_FILTERS*/
		UNION ALL
		SELECT t.con_id, t.owner, t.table_name, 'Y' AS is_object
		FROM cdb_object_tables t
		WHERE t.nested = 'NO'
			AND t.secondary = 'N'
			AND NVL(t.dropped, 'NO') = 'NO'
			AND (t.iot_type IS NULL OR t.iot_type = 'IOT')
			AND t.table_name NOT LIKE 'BIN$%'
			AND t.owner IN (/*OWNERS*/)
			/*TABLE_FILTERS*/
	)
),
ranked_columns AS (
	SELECT c.con_id, c.owner, c.table_name, c.column_name, c.column_id, c.internal_column_id,
		c.virtual_column, c.hidden_column, c.data_type, c.data_type_owner, c.data_type_mod,
		c.data_length, c.char_length, c.data_precision, c.data_scale, c.char_used, c.nullable,
		/*DEFAULT_COL*/ AS data_default_vc,
		ROW_NUMBER() OVER (PARTITION BY c.con_id, c.owner, c.table_name ORDER BY c.internal_column_id) AS col_rn,
		COUNT(*) OVER (PARTITION BY c.con_id, c.owner, c.table_name) AS total_columns
	FROM cdb_tab_cols c
	WHERE c.owner IN (/*OWNERS*/)
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
	rt.total_tables,
	c.column_name,
	c.column_id,
	c.internal_column_id,
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
	c.total_columns
FROM cdb_tables t
JOIN ranked_tables rt
	ON rt.con_id = t.con_id AND rt.owner = t.owner AND rt.table_name = t.table_name AND rt.is_object = 'N'
JOIN ranked_columns c
	ON c.con_id = t.con_id AND c.owner = t.owner AND c.table_name = t.table_name
WHERE rt.rn <= /*MAX_TABLES*/ AND c.col_rn <= /*MAX_COLUMNS*/
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
	rt.total_tables,
	c.column_name,
	c.column_id,
	c.internal_column_id,
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
	c.total_columns
FROM cdb_object_tables t
JOIN ranked_tables rt
	ON rt.con_id = t.con_id AND rt.owner = t.owner AND rt.table_name = t.table_name AND rt.is_object = 'Y'
JOIN ranked_columns c
	ON c.con_id = t.con_id AND c.owner = t.owner AND c.table_name = t.table_name
WHERE rt.rn <= /*MAX_TABLES*/ AND c.col_rn <= /*MAX_COLUMNS*/
ORDER BY con_id, owner, table_name, internal_column_id`

// 21c+ and separately granted, so a missing view or column is expected rather than fatal.
const blockchainTablesQuery = `SELECT con_id, schema_name, table_name, row_retention, row_retention_locked,
	table_inactivity_retention, hash_algorithm, table_version
FROM cdb_blockchain_tables WHERE schema_name IN (/*OWNERS*/)`

const immutableTablesQuery = `SELECT con_id, schema_name, table_name, row_retention, row_retention_locked,
	table_inactivity_retention
FROM cdb_immutable_tables WHERE schema_name IN (/*OWNERS*/)`

const tabModificationsQuery = `SELECT con_id, table_owner, table_name, inserts, updates, deletes, truncated, timestamp
FROM cdb_tab_modifications WHERE partition_name IS NULL AND table_owner IN (/*OWNERS*/)`

const partTablesQuery = `SELECT con_id, owner, table_name, partitioning_type, subpartitioning_type, partition_count
FROM cdb_part_tables WHERE owner IN (/*OWNERS*/)`

const partKeyColumnsQuery = `SELECT con_id, owner, name, column_name
FROM cdb_part_key_columns WHERE object_type = 'TABLE' AND owner IN (/*OWNERS*/)
ORDER BY con_id, owner, name, column_position`

const tableCommentsQuery = `SELECT con_id, owner, table_name, comments
FROM cdb_tab_comments WHERE comments IS NOT NULL AND owner IN (/*OWNERS*/)`

const columnCommentsQuery = `SELECT con_id, owner, table_name, column_name, comments
FROM cdb_col_comments WHERE comments IS NOT NULL AND owner IN (/*OWNERS*/)`

// A function-based index has no real column to key on: Oracle backs it with a hidden,
// system-generated SYS_NC%$ virtual column on the base table, and CDB_IND_COLUMNS carries only
// that generated name, not the expression. The expression itself lives where any virtual
// column's definition lives -- CDB_TAB_COLS' default-value column -- so the hidden column is
// joined back to its own table row to recover it. CDB_IND_EXPRESSIONS looks like the obvious
// source, but its CDB_ variant drops COLUMN_EXPRESSION entirely (unlike CDB_TAB_COLS, it was
// never given a LONG-safe replacement), so it cannot be used here.
const indexesQuery = `SELECT i.con_id, i.table_owner, i.table_name, i.index_name, i.uniqueness, i.index_type,
	ic.column_name, /*EXPRESSION_COL*/ AS column_expression
FROM cdb_indexes i
JOIN cdb_ind_columns ic
	ON ic.con_id = i.con_id AND ic.index_owner = i.owner AND ic.index_name = i.index_name
LEFT JOIN cdb_tab_cols tc
	ON tc.con_id = ic.con_id AND tc.owner = ic.table_owner AND tc.table_name = ic.table_name
	AND tc.column_name = ic.column_name AND ic.column_name LIKE 'SYS\_NC%' ESCAPE '\'
WHERE i.table_owner IN (/*OWNERS*/)
ORDER BY i.con_id, i.table_owner, i.table_name, i.index_name, ic.column_position`

// User-defined CHECK constraints (constraint_type = 'C') are collected alongside P/U/R;
// generated = 'USER NAME' excludes Oracle's own NOT NULL checks, which it implements as
// system-generated CHECK constraints and which would otherwise duplicate the column's nullable flag.
const constraintsQuery = `SELECT c.con_id, c.owner, c.table_name, c.constraint_name, c.constraint_type,
	NVL(c.r_owner, '-') AS r_owner, NVL(c.r_constraint_name, '-') AS r_constraint_name, cc.column_name,
	/*CONDITION_COL*/ AS search_condition
FROM cdb_constraints c
JOIN cdb_cons_columns cc
	ON cc.con_id = c.con_id AND cc.owner = c.owner AND cc.constraint_name = c.constraint_name
WHERE (c.constraint_type IN ('P', 'U', 'R') OR (c.constraint_type = 'C' AND c.generated = 'USER NAME'))
	AND c.owner IN (/*OWNERS*/)
ORDER BY c.con_id, c.owner, c.table_name, c.constraint_name, cc.position`

const externalTablesQuery = `SELECT con_id, owner, table_name, type_name, NVL(default_directory_name, '-')
FROM cdb_external_tables WHERE owner IN (/*OWNERS*/)`

const externalLocationsQuery = `SELECT con_id, owner, table_name, NVL(directory_name, '-'), location
FROM cdb_external_locations WHERE owner IN (/*OWNERS*/)`

// A materialized view's container table appears in CDB_TABLES under the mview name, so
// without this it is catalogued as an ordinary table.
const mviewsQuery = `SELECT con_id, owner, mview_name, NVL(refresh_mode, '-'), NVL(refresh_method, '-'),
	NVL(staleness, '-'), last_refresh_date
FROM cdb_mviews WHERE owner IN (/*OWNERS*/)`

const containerNamesQuery = `SELECT con_id, name FROM v$containers`

// Views reuse the table row shape so the same scanning and type rendering apply; the
// table-only columns are filled with the values an ordinary heap table would report.
//
// max_columns is enforced the same way as in schemasQueryTemplate: ranked_columns ranks
// cdb_tab_cols by internal_column_id within each view, and total_columns rides along per row.
const viewsQueryTemplate = `WITH ranked_columns AS (
	SELECT c.con_id, c.owner, c.table_name, c.column_name, c.column_id, c.internal_column_id,
		c.virtual_column, c.hidden_column, c.data_type, c.data_type_owner, c.data_type_mod,
		c.data_length, c.char_length, c.data_precision, c.data_scale, c.char_used, c.nullable,
		/*DEFAULT_COL*/ AS data_default_vc,
		ROW_NUMBER() OVER (PARTITION BY c.con_id, c.owner, c.table_name ORDER BY c.internal_column_id) AS col_rn,
		COUNT(*) OVER (PARTITION BY c.con_id, c.owner, c.table_name) AS total_columns
	FROM cdb_tab_cols c
	WHERE c.owner IN (/*OWNERS*/)
		AND NOT (c.hidden_column = 'YES' AND c.user_generated = 'NO')
)
SELECT
	v.con_id,
	v.owner,
	v.view_name AS table_name,
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
	c.total_columns
FROM cdb_views v
JOIN ranked_columns c
	ON c.con_id = v.con_id AND c.owner = v.owner AND c.table_name = v.view_name
WHERE v.owner IN (/*OWNERS*/)
	/*TABLE_FILTERS*/
	AND c.col_rn <= /*MAX_COLUMNS*/
ORDER BY v.con_id, v.owner, v.view_name, c.internal_column_id`

const viewDefinitionsQuery = `SELECT con_id, owner, view_name, text_vc
FROM cdb_views WHERE owner IN (/*OWNERS*/)`

const viewObjectsQuery = `SELECT con_id, owner, object_name, object_id, created, last_ddl_time
FROM cdb_objects WHERE object_type = 'VIEW' AND owner IN (/*OWNERS*/)`

// ORA-01795 caps a single IN-list expression at 1000 items, so owner names beyond that must be
// queried in separate batches and the results unioned.
const maxSchemaOwners = 1000

var schemaOwnerPattern = regexp.MustCompile(`^[A-Z0-9_$#]+$`)

// compiledPatterns compiles each of patterns as a regexp, matching the POSIX-ERE semantics of
// Postgres's include/exclude filters (see filters.py). An invalid pattern is dropped rather than
// failing the whole collection.
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

// passesFilter mirrors Postgres's precedence: exclude wins outright, and when include is
// non-empty at least one include pattern must match.
func passesFilter(name string, include, exclude []*regexp.Regexp) bool {
	if matchesAny(name, exclude) {
		return false
	}
	return len(include) == 0 || matchesAny(name, include)
}

// escapeSQLLiteral doubles single quotes so a config-supplied regex pattern can be embedded as a
// SQL string literal. Patterns are check config, not user input reachable at runtime, but this
// keeps a stray quote in a pattern from producing a broken statement instead of a broken filter.
func escapeSQLLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// regexSQLClauses renders include/exclude filters as REGEXP_LIKE predicates against column,
// applied with the same precedence as passesFilter: exclude clauses are AND NOT'd in first, then
// an OR'd include clause is required to also match.
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

// filterContainers drops containers whose name fails the database include/exclude filters. An
// Oracle "database" for this purpose is a container (CDB root or PDB).
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

type tableKey struct {
	conID int64
	owner string
	table string
}

type schemaCollector struct {
	check      *Check
	kind       string
	emit       payloadEmitter
	details    map[tableKey]*tableDetails
	views      map[tableKey]*viewDetails
	owners     map[ownerKey]string
	containers map[int64]string
	started    map[int64]struct{}

	conID       int64
	conName     string
	startedAt   int64
	payloads    int
	schemas     []*schemaObject
	tableCount  int
	tablesTotal int

	// containerCount and truncated reset per container (in startContainer), unlike tablesTotal
	// which accumulates for the whole run; max_views is a per-container cap, so the count it is
	// compared against must be too.
	containerCount int
	truncated      bool

	currentSchema *schemaObject
	currentTable  *schemaTable
	currentView   *viewObject
}

func newSchemaCollector(c *Check, emit payloadEmitter, details map[tableKey]*tableDetails, owners map[ownerKey]string, containers map[int64]string) *schemaCollector {
	return &schemaCollector{check: c, kind: "oracle_databases", emit: emit, details: details, owners: owners, containers: containers, conID: -1, started: make(map[int64]struct{})}
}

func newViewCollector(c *Check, emit payloadEmitter, views map[tableKey]*viewDetails, owners map[ownerKey]string, containers map[int64]string) *schemaCollector {
	return &schemaCollector{check: c, kind: "oracle_views", emit: emit, views: views, owners: owners, containers: containers, conID: -1, started: make(map[int64]struct{})}
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
	s.containerCount = 0
	s.truncated = false
	s.reset()
}

// nextSnapshotID returns a millisecond timestamp that never repeats for this check.
// collection_started_at is the only thing distinguishing one snapshot from another, and
// containers and kinds are collected fast enough to share a millisecond otherwise.
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
	s.currentView = nil
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
	if !isLast && s.tableCount < s.check.config.Schemas.PayloadChunkSize {
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

	payloadBytes, err := json.Marshal(e)
	if err != nil {
		log.Errorf("%s failed to marshal schema payload: %s", s.check.logPrompt, err)
		return
	}
	s.emit(payloadBytes)
	log.Debugf("%s schema payload con_id=%d tables=%d bytes=%d last=%t",
		s.check.logPrompt, s.conID, s.tableCount, len(payloadBytes), isLast)

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
	s.currentView = nil
}

// addView mirrors add, but the row describes a view: same envelope, same chunking, and the
// payload hangs off the schema's views list instead of its tables list.
func (s *schemaCollector) addView(r schemaRowDB) {
	if r.ConID != s.conID {
		s.startContainer(r.ConID)
	}

	// The flush check must happen at the view boundary, before the new view is appended --
	// flushing after a column append can split a view's columns across two payloads.
	newView := s.currentSchema == nil || s.currentSchema.Name != r.Owner || s.currentView == nil || s.currentView.Name != r.TableName
	if newView {
		s.maybeFlush(false)
	}

	s.useSchema(r.ConID, r.Owner)

	if newView {
		v := &viewObject{Name: r.TableName, Owner: r.Owner}
		if d := s.views[tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName}]; d != nil {
			v.ID = d.ID
			v.Definition = d.Definition
			v.Comment = d.Comment
			v.CreateDate = d.CreateDate
			v.ModifyDate = d.ModifyDate
		}
		s.currentSchema.Views = append(s.currentSchema.Views, v)
		s.currentView = v
		s.tableCount++
		s.tablesTotal++
		s.containerCount++

		// TOTAL_COLUMNS is the ranked_columns window total for this view, computed before
		// max_columns truncates it; same reasoning as TOTAL_TABLES in add(), one level down.
		if r.TotalColumns.Valid && r.TotalColumns.Int64 > int64(s.check.config.Schemas.MaxColumns) {
			s.truncated = true
		}
	}

	col := schemaColumn{
		Name:      r.ColumnName,
		DataType:  dataType(r),
		Nullable:  r.Nullable == "Y",
		Virtual:   r.VirtualColumn == "YES",
		Invisible: r.HiddenColumn == "YES",
	}
	s.currentView.Columns = append(s.currentView.Columns, col)
}

func (s *schemaCollector) add(r schemaRowDB) {
	if r.ConID != s.conID {
		s.startContainer(r.ConID)
	}

	// The flush check must happen at the table boundary, before the new table is appended --
	// flushing after a column append can split a table's columns across two payloads, and the
	// backend has no merge semantics for a table that reappears with the same id.
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
		if d := s.details[tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName}]; d != nil || r.NumRows.Valid {
			// NUM_ROWS is the estimate as of LAST_ANALYZED; the modification counters are the
			// deltas accumulated since, and Oracle drops them at the next gather.
			var estimate int64
			known := false
			if r.NumRows.Valid {
				estimate = r.NumRows.Int64
				known = true
			}
			if d != nil && d.Modifications != nil {
				estimate += d.Modifications.Inserts - d.Modifications.Deletes
				known = true
			}
			if known {
				if estimate < 0 {
					estimate = 0
				}
				t.RowCount = &estimate
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
			t.Modifications = d.Modifications
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
		s.containerCount++

		// TOTAL_TABLES is the ranked_tables window total for this container, computed before
		// max_tables truncates it; a table already present is by definition within the cap, so
		// this only ever flips true, and it does so from the very first row of the container.
		if r.TotalTables.Valid && r.TotalTables.Int64 > int64(s.check.config.Schemas.MaxTables) {
			s.truncated = true
		}
		// TOTAL_COLUMNS is the ranked_columns window total for this table, computed before
		// max_columns truncates it; same reasoning as TOTAL_TABLES above, one level down.
		if r.TotalColumns.Valid && r.TotalColumns.Int64 > int64(s.check.config.Schemas.MaxColumns) {
			s.truncated = true
		}
	}

	col := schemaColumn{
		Name:      r.ColumnName,
		DataType:  dataType(r),
		Nullable:  r.Nullable == "Y",
		Virtual:   r.VirtualColumn == "YES",
		Invisible: r.HiddenColumn == "YES",
	}
	if r.DataDefault.Valid {
		col.Default = truncateLongValue(r.DataDefault.String)
	}
	if d := s.details[tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName}]; d != nil {
		col.Comment = d.ColumnComments[r.ColumnName]
	}
	s.currentTable.Columns = append(s.currentTable.Columns, col)
}

// finish flushes the final, terminating payload for the active container. It must only be
// called on the success path: it is what tells the backend a snapshot is complete, so calling
// it after a mid-collection error would mark a truncated snapshot as whole.
func (s *schemaCollector) finish() {
	if s.conID == -1 {
		return
	}
	s.maybeFlush(true)
	s.conID = -1
}

// emitEmptyContainers sends a terminating, empty-metadata payload for every container that
// never produced a row (e.g. a PDB whose user tables were all dropped), so the backend learns
// it is empty instead of keeping its last snapshot forever.
func (s *schemaCollector) emitEmptyContainers(containers map[int64]string) {
	for conID := range containers {
		if _, ok := s.started[conID]; ok {
			continue
		}
		s.startContainer(conID)
		s.finish()
	}
}

// Oracle has no format_type(): length, precision and scale arrive as separate columns.
// DATA_LENGTH is always bytes, so character semantics must use CHAR_LENGTH -- a
// VARCHAR2(20 CHAR) column reports DATA_LENGTH 80 on AL32UTF8.
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
		// Temporal types already carry their precision inside DATA_TYPE, and DATA_LENGTH is
		// an internal locator size for LOB, object, XML and JSON columns.
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

// Oracle's table attributes compound -- a table can be temporary and partitioned and
// clustered at once -- so they travel as a set rather than collapsing into table_type.
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

// DATA_DEFAULT_VC only exists from 23ai; DATA_DEFAULT (a LONG) covers every earlier version.
// Oracle's LONG restrictions forbid a LONG in WHERE, GROUP BY, DISTINCT, ORDER BY, a function
// call, or a UNION -- none of which apply to a plain select-list column in this joined,
// UNION ALL query, so selecting it directly is safe. The VARCHAR2(4000) truncation is applied
// in Go (see truncateLongValue) since running a function on the LONG column itself would fall
// under the "function" restriction above.
func (c *Check) defaultValueColumn() string {
	major, _, _ := strings.Cut(c.dbVersion, ".")
	if n, err := strconv.Atoi(major); err == nil && n >= 23 {
		return "c.data_default_vc"
	}
	return "c.data_default"
}

// SEARCH_CONDITION_VC only exists from 12c; before that only the LONG SEARCH_CONDITION is
// available, and it is safe to select directly for the same reason as DATA_DEFAULT above.
func (c *Check) conditionColumn() string {
	major, _, _ := strings.Cut(c.dbVersion, ".")
	if n, err := strconv.Atoi(major); err == nil && n >= 12 {
		return "c.search_condition_vc"
	}
	return "c.search_condition"
}

// indexExpressionColumn mirrors defaultValueColumn: same _VC cutover, same LONG-selection
// reasoning, but keyed off the tc alias (cdb_tab_cols joined back from cdb_ind_columns in
// indexesQuery) rather than ranked_columns' c.
func (c *Check) indexExpressionColumn() string {
	major, _, _ := strings.Cut(c.dbVersion, ".")
	if n, err := strconv.Atoi(major); err == nil && n >= 23 {
		return "tc.data_default_vc"
	}
	return "tc.data_default"
}

// truncateLongValue bounds a value read from a LONG column (DATA_DEFAULT, SEARCH_CONDITION) to
// VARCHAR2(4000)'s cap, so pre-23ai/pre-12c (LONG) and their _VC replacements report a
// consistently sized value. Truncating by rune rather than byte avoids splitting a multi-byte
// character in half.
func truncateLongValue(s string) string {
	r := []rune(s)
	if len(r) <= 4000 {
		return s
	}
	return string(r[:4000])
}

type ownerKey struct {
	conID int64
	owner string
}

// schemaOwners returns the schemas (Oracle "owners") to collect, restricted to containers and
// filtered by include_schemas/exclude_schemas before the names are ever quoted into a query.
func (c *Check) schemaOwners(ctx context.Context, containers map[int64]string) (map[ownerKey]string, []string, error) {
	rows, err := c.db.QueryxContext(ctx, schemaOwnersQuery)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query schema owners: %w", err)
	}
	defer rows.Close()

	include := compiledPatterns(c.config.Schemas.IncludeSchemas, c.logPrompt, "include_schemas")
	exclude := compiledPatterns(c.config.Schemas.ExcludeSchemas, c.logPrompt, "exclude_schemas")
	// Only gate on container membership when the user actually configured a database filter;
	// otherwise a container lookup that fails (or simply lags behind cdb_users) must not silently
	// drop every schema.
	filterDatabases := len(c.config.Schemas.IncludeDatabases) > 0 || len(c.config.Schemas.ExcludeDatabases) > 0

	owners := make(map[ownerKey]string)
	names := make(map[string]struct{})
	for rows.Next() {
		var (
			conID  int64
			name   string
			userID sql.NullInt64
		)
		if err := rows.Scan(&conID, &name, &userID); err != nil {
			return nil, nil, fmt.Errorf("failed to scan schema owner: %w", err)
		}
		if filterDatabases {
			if _, ok := containers[conID]; !ok {
				continue
			}
		}
		if !schemaOwnerPattern.MatchString(name) {
			log.Warnf("%s skipping schema owner with unexpected characters: %q", c.logPrompt, name)
			continue
		}
		if !passesFilter(name, include, exclude) {
			continue
		}
		id := ""
		if userID.Valid {
			id = strconv.FormatInt(userID.Int64, 10)
		}
		owners[ownerKey{conID: conID, owner: name}] = id
		names[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	distinct := make([]string, 0, len(names))
	for n := range names {
		distinct = append(distinct, n)
	}
	sort.Strings(distinct)
	return owners, distinct, nil
}

// ownerListChunks renders names as one or more quoted, comma-joined IN-list bodies, split into
// batches of maxSchemaOwners (see its comment for why).
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

// A view that is absent (ORA-00942) or shaped differently (ORA-00904) means the property is
// unavailable on this version or not granted; the rest of the collection still stands.
//
// ownerLists holds one or more IN-list batches (see maxSchemaOwners); the query runs once per
// batch and every batch feeds the same scan callback, so the results end up unioned.
func (c *Check) queryDetails(ctx context.Context, name, template string, ownerLists []string, scan func(*sqlx.Rows) error) {
	for _, ownerList := range ownerLists {
		rows, err := c.db.QueryxContext(ctx, strings.Replace(template, "/*OWNERS*/", ownerList, 1))
		if err != nil {
			if strings.Contains(err.Error(), "ORA-00942") || strings.Contains(err.Error(), "ORA-00904") {
				log.Debugf("%s table detail %q unavailable: %s", c.logPrompt, name, err)
				return
			}
			log.Warnf("%s failed to collect table detail %q: %s", c.logPrompt, name, err)
			return
		}
		for rows.Next() {
			if err := scan(rows); err != nil {
				log.Warnf("%s failed to scan table detail %q: %s", c.logPrompt, name, err)
				rows.Close()
				return
			}
		}
		rows.Close()
	}
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

func (c *Check) containerNames(ctx context.Context) map[int64]string {
	names := make(map[int64]string)
	rows, err := c.db.QueryxContext(ctx, containerNamesQuery)
	if err != nil {
		log.Warnf("%s failed to query container names: %s", c.logPrompt, err)
		return names
	}
	defer rows.Close()
	for rows.Next() {
		var (
			conID int64
			name  string
		)
		if err := rows.Scan(&conID, &name); err != nil {
			log.Warnf("%s failed to scan container name: %s", c.logPrompt, err)
			return names
		}
		names[conID] = name
	}
	return names
}

// tableDetails runs the detail queries scoped to ownerList (the owner batches, same as the main
// query), but only ever materializes an entry for a table in allowed -- the capped, filtered set
// that the main query actually returned. A row for a table outside that set lands in a throwaway
// value instead, so the resident map stays bounded by max_tables rather than by the size of the
// full owner-scoped catalog these detail queries still read from the server.
func (c *Check) tableDetails(ctx context.Context, ownerList []string, allowed map[tableKey]struct{}) map[tableKey]*tableDetails {
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

	c.queryDetails(ctx, "blockchain", blockchainTablesQuery, ownerList, scanRetention(true))
	c.queryDetails(ctx, "immutable", immutableTablesQuery, ownerList, scanRetention(false))

	c.queryDetails(ctx, "modifications", tabModificationsQuery, ownerList, func(rows *sqlx.Rows) error {
		var (
			conID                     int64
			owner, table              string
			inserts, updates, deletes sql.NullInt64
			truncated                 sql.NullString
			ts                        sql.NullTime
		)
		if err := rows.Scan(&conID, &owner, &table, &inserts, &updates, &deletes, &truncated, &ts); err != nil {
			return err
		}
		d := &modificationsDetail{
			Inserts:   inserts.Int64,
			Updates:   updates.Int64,
			Deletes:   deletes.Int64,
			Truncated: truncated.String == "YES",
		}
		if ts.Valid {
			d.LastModified = ts.Time.UTC().Format(time.RFC3339)
		}
		at(conID, owner, table).Modifications = d
		return nil
	})

	c.queryDetails(ctx, "materialized views", mviewsQuery, ownerList, func(rows *sqlx.Rows) error {
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

	c.queryDetails(ctx, "table comments", tableCommentsQuery, ownerList, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, comment string
		if err := rows.Scan(&conID, &owner, &table, &comment); err != nil {
			return err
		}
		at(conID, owner, table).Comment = comment
		return nil
	})

	c.queryDetails(ctx, "column comments", columnCommentsQuery, ownerList, func(rows *sqlx.Rows) error {
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

	indexesQueryResolved := strings.Replace(indexesQuery, "/*EXPRESSION_COL*/", c.indexExpressionColumn(), 1)
	c.queryDetails(ctx, "indexes", indexesQueryResolved, ownerList, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, name, uniqueness, indexType, column string
		var expression sql.NullString
		if err := rows.Scan(&conID, &owner, &table, &name, &uniqueness, &indexType, &column, &expression); err != nil {
			return err
		}
		d := at(conID, owner, table)
		var idx *indexInfo
		if n := len(d.Indexes); n > 0 && d.Indexes[n-1].Name == name {
			idx = d.Indexes[n-1]
		} else {
			idx = &indexInfo{Name: name, Unique: uniqueness == "UNIQUE", Type: indexType}
			d.Indexes = append(d.Indexes, idx)
		}
		// A function-based index's key column is the hidden SYS_NC%$ virtual column joined in
		// by indexesQuery; report its expression instead of that meaningless generated name.
		if expression.Valid {
			idx.Columns = append(idx.Columns, indexKeyPart{Expression: truncateLongValue(expression.String)})
		} else {
			idx.Columns = append(idx.Columns, indexKeyPart{Column: column})
		}
		return nil
	})

	// Referenced tables are resolved after the scan: a foreign key names the constraint it
	// points at, not the table, and that constraint is usually on another table in this set.
	primaryKeys := make(map[string]*constraintInfo)
	constraintsQueryResolved := strings.Replace(constraintsQuery, "/*CONDITION_COL*/", c.conditionColumn(), 1)
	c.queryDetails(ctx, "constraints", constraintsQueryResolved, ownerList, func(rows *sqlx.Rows) error {
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
				con.referencedKey = fmt.Sprintf("%d|%s|%s", conID, rOwner, rName)
				if rName != "-" {
					con.referencedName = rName
				}
			}
			d.Constraints = append(d.Constraints, con)
		}
		con.Columns = append(con.Columns, column)
		if ctype == "P" || ctype == "U" {
			primaryKeys[fmt.Sprintf("%d|%s|%s", conID, owner, name)] = &constraintInfo{
				ReferencedTable:   table,
				ReferencedColumns: con.Columns,
			}
		}
		return nil
	})
	for _, d := range details {
		for _, con := range d.Constraints {
			if con.referencedKey == "" {
				continue
			}
			if target, ok := primaryKeys[con.referencedKey]; ok {
				con.ReferencedTable = target.ReferencedTable
				con.ReferencedColumns = target.ReferencedColumns
			} else {
				// The referenced owner isn't among the collected schemas, so the target table
				// can't be resolved; fall back to naming the constraint rather than leaving
				// ReferencedTable/ReferencedColumns empty, which would read as corrupt data.
				con.ReferencedConstraint = con.referencedName
			}
		}
	}

	c.queryDetails(ctx, "external tables", externalTablesQuery, ownerList, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, driver, directory string
		if err := rows.Scan(&conID, &owner, &table, &driver, &directory); err != nil {
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
		return nil
	})

	c.queryDetails(ctx, "external locations", externalLocationsQuery, ownerList, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, directory, location string
		if err := rows.Scan(&conID, &owner, &table, &directory, &location); err != nil {
			return err
		}
		d := at(conID, owner, table)
		if d.External == nil {
			d.External = &externalDetail{}
		}
		if directory != "-" {
			location = directory + ":" + location
		}
		d.External.Locations = append(d.External.Locations, location)
		return nil
	})

	c.queryDetails(ctx, "object ids", objectIDsQuery, ownerList, func(rows *sqlx.Rows) error {
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

	c.queryDetails(ctx, "partitioning", partTablesQuery, ownerList, func(rows *sqlx.Rows) error {
		var (
			conID          int64
			owner, table   string
			ptype, subtype sql.NullString
			count          sql.NullInt64
		)
		if err := rows.Scan(&conID, &owner, &table, &ptype, &subtype, &count); err != nil {
			return err
		}
		d := &partitionDetail{PartitioningType: ptype.String, NumPartitions: count.Int64}
		if subtype.Valid && subtype.String != "NONE" {
			d.SubpartitionsType = subtype.String
		}
		at(conID, owner, table).Partitioned = d
		return nil
	})

	keys := make(map[tableKey][]string)
	c.queryDetails(ctx, "partition keys", partKeyColumnsQuery, ownerList, func(rows *sqlx.Rows) error {
		var (
			conID             int64
			owner, table, col string
		)
		if err := rows.Scan(&conID, &owner, &table, &col); err != nil {
			return err
		}
		k := tableKey{conID: conID, owner: owner, table: table}
		keys[k] = append(keys[k], col)
		return nil
	})
	for k, cols := range keys {
		if d := details[k]; d != nil && d.Partitioned != nil {
			d.Partitioned.PartitionKey = fmt.Sprintf("%s (%s)", d.Partitioned.PartitioningType, strings.Join(cols, ", "))
		}
	}

	return details
}

// viewDetails mirrors tableDetails: only a view in allowed (the post-max_views set) gets an
// entry in the resident map.
func (c *Check) viewDetails(ctx context.Context, ownerList []string, allowed map[tableKey]struct{}) map[tableKey]*viewDetails {
	details := make(map[tableKey]*viewDetails)
	at := func(conID int64, owner, name string) *viewDetails {
		k := tableKey{conID: conID, owner: owner, table: name}
		if _, ok := allowed[k]; !ok {
			return &viewDetails{}
		}
		if details[k] == nil {
			details[k] = &viewDetails{}
		}
		return details[k]
	}

	c.queryDetails(ctx, "view definitions", viewDefinitionsQuery, ownerList, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, name string
		var text sql.NullString
		if err := rows.Scan(&conID, &owner, &name, &text); err != nil {
			return err
		}
		at(conID, owner, name).Definition = text.String
		return nil
	})

	c.queryDetails(ctx, "view objects", viewObjectsQuery, ownerList, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, name string
		var objectID sql.NullInt64
		var created, lastDDL sql.NullTime
		if err := rows.Scan(&conID, &owner, &name, &objectID, &created, &lastDDL); err != nil {
			return err
		}
		d := at(conID, owner, name)
		if objectID.Valid {
			d.ID = strconv.FormatInt(objectID.Int64, 10)
		}
		if created.Valid {
			d.CreateDate = created.Time.UTC().Format(time.RFC3339)
		}
		if lastDDL.Valid {
			d.ModifyDate = lastDDL.Time.UTC().Format(time.RFC3339)
		}
		return nil
	})

	c.queryDetails(ctx, "view comments", tableCommentsQuery, ownerList, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, name, comment string
		if err := rows.Scan(&conID, &owner, &name, &comment); err != nil {
			return err
		}
		if d, ok := details[tableKey{conID: conID, owner: owner, table: name}]; ok {
			d.Comment = comment
		}
		return nil
	})

	return details
}

// fetchMetadataRows runs a table/view query once per owner batch (see maxSchemaOwners) and
// returns the rows belonging to a collected owner. A single batch already comes back from
// Oracle sorted by con_id; with more than one batch that ordering only holds within each batch,
// so the combined rows are regrouped by con_id to keep every container's rows contiguous --
// required so the collector never revisits a container it has already flushed as complete.
//
// This buffering is also what guarantees a snapshot is never emitted incomplete: every row is
// read into memory here before a schemaCollector exists, so a query or scan error returns to the
// caller before any payload has been built, let alone sent. That is why the error paths below
// have no explicit cleanup -- there is nothing yet to clean up.
func (c *Check) fetchMetadataRows(ctx context.Context, template string, ownerLists []string, owners map[ownerKey]string, extra map[string]string) ([]schemaRowDB, error) {
	var all []schemaRowDB
	for _, ownerList := range ownerLists {
		// The cdb_tables and cdb_object_tables branches of the schema query each carry their
		// own copy of every placeholder, so every occurrence must be substituted, not just the
		// first.
		query := strings.ReplaceAll(template, "/*OWNERS*/", ownerList)
		query = strings.ReplaceAll(query, "/*DEFAULT_COL*/", c.defaultValueColumn())
		for placeholder, value := range extra {
			query = strings.ReplaceAll(query, placeholder, value)
		}

		rows, err := c.db.QueryxContext(ctx, query)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var r schemaRowDB
			if err := rows.StructScan(&r); err != nil {
				rows.Close()
				return nil, err
			}
			if _, ok := owners[ownerKey{conID: r.ConID, owner: r.Owner}]; ok {
				all = append(all, r)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	if len(ownerLists) > 1 {
		sort.SliceStable(all, func(i, j int) bool { return all[i].ConID < all[j].ConID })
	}
	return all, nil
}

// tableKeysFromRows collects the distinct tables a metadata query actually returned, used to
// bound a details map to the same capped, filtered set.
func tableKeysFromRows(rows []schemaRowDB) map[tableKey]struct{} {
	keys := make(map[tableKey]struct{}, len(rows))
	for _, r := range rows {
		keys[tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName}] = struct{}{}
	}
	return keys
}

// viewKeysWithCap applies max_views per container -- rows arrive grouped contiguously by
// container (see fetchMetadataRows) -- and reports which container hit the cap, so the caller
// can mark its payload truncated instead of just dropping the excess silently.
func viewKeysWithCap(rows []schemaRowDB, maxViews int) (map[tableKey]struct{}, map[int64]bool) {
	allowed := make(map[tableKey]struct{})
	truncated := make(map[int64]bool)
	counts := make(map[int64]int)
	for _, r := range rows {
		k := tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName}
		if _, ok := allowed[k]; ok {
			continue
		}
		if maxViews > 0 && counts[r.ConID] >= maxViews {
			truncated[r.ConID] = true
			continue
		}
		counts[r.ConID]++
		allowed[k] = struct{}{}
	}
	return allowed, truncated
}

// ViewCollection emits view metadata as its own kind, mirroring how sqlserver_views is kept
// separate from sqlserver_databases.
func (c *Check) ViewCollection(ctx context.Context, owners map[ownerKey]string, names []string, containers map[int64]string) error {
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}

	ownerLists := ownerListChunks(names)

	extra := map[string]string{
		"/*TABLE_FILTERS*/": regexSQLClauses("v.view_name", c.config.Schemas.IncludeTables, c.config.Schemas.ExcludeTables),
		"/*MAX_COLUMNS*/":   strconv.Itoa(c.config.Schemas.MaxColumns),
	}
	rows, err := c.fetchMetadataRows(ctx, viewsQueryTemplate, ownerLists, owners, extra)
	if err != nil {
		return fmt.Errorf("failed to query views: %w", err)
	}

	allowedViews, truncatedContainers := viewKeysWithCap(rows, c.config.Schemas.MaxViews)

	collector := newViewCollector(c, func(payload []byte) {
		sender.EventPlatformEvent(payload, "dbm-metadata")
	}, c.viewDetails(ctx, ownerLists, allowedViews), owners, containers)

	for _, r := range rows {
		if r.ConID != collector.conID {
			collector.startContainer(r.ConID)
			collector.truncated = truncatedContainers[r.ConID]
		}
		k := tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName}
		isNewView := collector.currentSchema == nil || collector.currentSchema.Name != r.Owner ||
			collector.currentView == nil || collector.currentView.Name != r.TableName
		if isNewView {
			if _, ok := allowedViews[k]; !ok {
				continue
			}
		}
		collector.addView(r)
	}
	collector.finish()
	collector.emitEmptyContainers(containers)

	for conID := range truncatedContainers {
		log.Warnf("%s view collection stopped at max_views=%d for container %d; some views were not collected",
			c.logPrompt, c.config.Schemas.MaxViews, conID)
	}
	log.Debugf("%s view collection sent %d views", c.logPrompt, collector.tablesTotal)
	return nil
}

// SchemaCollection streams table and column metadata and emits it as dbm-metadata payloads,
// one snapshot per container.
func (c *Check) SchemaCollection() error {
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.config.Schemas.MaxQueryDurationDuration())
	defer cancel()

	containers := filterContainers(c.containerNames(ctx), c.config.Schemas.IncludeDatabases, c.config.Schemas.ExcludeDatabases, c.logPrompt)

	owners, names, err := c.schemaOwners(ctx, containers)
	if err != nil {
		return err
	}

	emit := func(payload []byte) {
		sender.EventPlatformEvent(payload, "dbm-metadata")
	}

	if len(names) == 0 {
		// No user schemas anywhere on the instance: every known container that previously held
		// a snapshot must still hear that it is now empty, or the backend keeps serving its
		// last snapshot forever.
		log.Debugf("%s no user schemas to collect, sending empty snapshot", c.logPrompt)
		newSchemaCollector(c, emit, nil, owners, containers).emitEmptyContainers(containers)
		if c.config.Schemas.ViewsEnabled() {
			newViewCollector(c, emit, nil, owners, containers).emitEmptyContainers(containers)
		}
		sender.Commit()
		return nil
	}

	ownerLists := ownerListChunks(names)

	extra := map[string]string{
		"/*TABLE_FILTERS*/": regexSQLClauses("t.table_name", c.config.Schemas.IncludeTables, c.config.Schemas.ExcludeTables),
		"/*MAX_TABLES*/":    strconv.Itoa(c.config.Schemas.MaxTables),
		"/*MAX_COLUMNS*/":   strconv.Itoa(c.config.Schemas.MaxColumns),
	}
	rows, err := c.fetchMetadataRows(ctx, schemasQueryTemplate, ownerLists, owners, extra)
	if err != nil {
		return fmt.Errorf("failed to query schemas: %w", err)
	}
	details := c.tableDetails(ctx, ownerLists, tableKeysFromRows(rows))

	collector := newSchemaCollector(c, emit, details, owners, containers)
	for _, r := range rows {
		collector.add(r)
	}
	collector.finish()
	collector.emitEmptyContainers(containers)

	log.Debugf("%s schema collection sent %d tables", c.logPrompt, collector.tablesTotal)

	if c.config.Schemas.ViewsEnabled() {
		// Views need a grant that table collection does not, so treat them as enrichment:
		// losing them must not discard the tables already emitted above.
		if err := c.ViewCollection(ctx, owners, names, containers); err != nil {
			log.Warnf("%s view collection failed, continuing without views: %s", c.logPrompt, err)
		}
	}

	sender.Commit()
	return nil
}
