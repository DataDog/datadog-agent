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
//
// Minimal version: tables and columns only.

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
const schemasQueryTemplate = `SELECT
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
	/*DEFAULT_COL*/ AS data_default_vc
FROM cdb_tables t
JOIN cdb_tab_cols c
	ON c.con_id = t.con_id AND c.owner = t.owner AND c.table_name = t.table_name
	AND NOT (c.hidden_column = 'YES' AND c.user_generated = 'NO')
WHERE t.nested = 'NO'
	AND t.secondary = 'N'
	AND NVL(t.dropped, 'NO') = 'NO'
	AND (t.iot_type IS NULL OR t.iot_type = 'IOT')
	AND t.table_name NOT LIKE 'BIN$%'
	AND t.owner IN (/*OWNERS*/)
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
	/*DEFAULT_COL*/ AS data_default_vc
FROM cdb_object_tables t
JOIN cdb_tab_cols c
	ON c.con_id = t.con_id AND c.owner = t.owner AND c.table_name = t.table_name
	AND NOT (c.hidden_column = 'YES' AND c.user_generated = 'NO')
WHERE t.nested = 'NO'
	AND t.secondary = 'N'
	AND NVL(t.dropped, 'NO') = 'NO'
	AND (t.iot_type IS NULL OR t.iot_type = 'IOT')
	AND t.table_name NOT LIKE 'BIN$%'
	AND t.owner IN (/*OWNERS*/)
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

const indexesQuery = `SELECT i.con_id, i.table_owner, i.table_name, i.index_name, i.uniqueness, i.index_type,
	ic.column_name
FROM cdb_indexes i
JOIN cdb_ind_columns ic
	ON ic.con_id = i.con_id AND ic.index_owner = i.owner AND ic.index_name = i.index_name
WHERE i.table_owner IN (/*OWNERS*/)
	AND ic.column_name NOT LIKE 'SYS\_NC%' ESCAPE '\'
ORDER BY i.con_id, i.table_owner, i.table_name, i.index_name, ic.column_position`

// System-generated NOT NULL checks are excluded: nullability is already on the column.
const constraintsQuery = `SELECT c.con_id, c.owner, c.table_name, c.constraint_name, c.constraint_type,
	NVL(c.r_owner, '-') AS r_owner, NVL(c.r_constraint_name, '-') AS r_constraint_name, cc.column_name
FROM cdb_constraints c
JOIN cdb_cons_columns cc
	ON cc.con_id = c.con_id AND cc.owner = c.owner AND cc.constraint_name = c.constraint_name
WHERE c.constraint_type IN ('P', 'U', 'R')
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
const viewsQueryTemplate = `SELECT
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
	/*DEFAULT_COL*/ AS data_default_vc
FROM cdb_views v
JOIN cdb_tab_cols c
	ON c.con_id = v.con_id AND c.owner = v.owner AND c.table_name = v.view_name
	AND NOT (c.hidden_column = 'YES' AND c.user_generated = 'NO')
WHERE v.owner IN (/*OWNERS*/)
ORDER BY v.con_id, v.owner, v.view_name, c.internal_column_id`

const viewDefinitionsQuery = `SELECT con_id, owner, view_name, text_vc
FROM cdb_views WHERE owner IN (/*OWNERS*/)`

const viewObjectsQuery = `SELECT con_id, owner, object_name, object_id, created, last_ddl_time
FROM cdb_objects WHERE object_type = 'VIEW' AND owner IN (/*OWNERS*/)`

const maxSchemaOwners = 1000

var schemaOwnerPattern = regexp.MustCompile(`^[A-Z0-9_$#]+$`)

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

type indexInfo struct {
	Name    string   `json:"name"`
	Unique  bool     `json:"unique"`
	Type    string   `json:"index_type,omitempty"`
	Columns []string `json:"columns,omitempty"`
}

type constraintInfo struct {
	Name              string   `json:"name"`
	Type              string   `json:"type"`
	Columns           []string `json:"columns,omitempty"`
	ReferencedOwner   string   `json:"referenced_owner,omitempty"`
	ReferencedTable   string   `json:"referenced_table,omitempty"`
	ReferencedColumns []string `json:"referenced_columns,omitempty"`
	referencedKey     string
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

// objectTypeDetail names the object type backing an object table (CREATE TABLE t OF type_t).
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

	conID       int64
	conName     string
	startedAt   int64
	payloads    int
	schemas     []*schemaObject
	tableCount  int
	tablesTotal int

	currentSchema *schemaObject
	currentTable  *schemaTable
	currentView   *viewObject
}

func newSchemaCollector(c *Check, emit payloadEmitter, details map[tableKey]*tableDetails, owners map[ownerKey]string, containers map[int64]string) *schemaCollector {
	return &schemaCollector{check: c, kind: "oracle_databases", emit: emit, details: details, owners: owners, containers: containers, conID: -1}
}

func newViewCollector(c *Check, emit payloadEmitter, views map[tableKey]*viewDetails, owners map[ownerKey]string, containers map[int64]string) *schemaCollector {
	return &schemaCollector{check: c, kind: "oracle_views", emit: emit, views: views, owners: owners, containers: containers, conID: -1}
}

func (s *schemaCollector) startContainer(conID int64) {
	if s.conID != -1 {
		s.maybeFlush(true)
	}
	s.conID = conID
	if name, ok := s.containers[conID]; ok {
		s.conName = s.check.getFullPDBName(name)
	} else {
		s.conName = s.check.getFullPDBName(strconv.FormatInt(conID, 10))
	}
	s.startedAt = s.check.nextSnapshotID()
	s.payloads = 0
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
	s.useSchema(r.ConID, r.Owner)

	if s.currentView == nil || s.currentView.Name != r.TableName {
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
	}

	col := schemaColumn{
		Name:      r.ColumnName,
		DataType:  dataType(r),
		Nullable:  r.Nullable == "Y",
		Virtual:   r.VirtualColumn == "YES",
		Invisible: r.HiddenColumn == "YES",
	}
	s.currentView.Columns = append(s.currentView.Columns, col)

	s.maybeFlush(false)
}

func (s *schemaCollector) add(r schemaRowDB) {
	if r.ConID != s.conID {
		s.startContainer(r.ConID)
	}

	s.useSchema(r.ConID, r.Owner)

	if s.currentTable == nil || s.currentTable.Name != r.TableName {
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
	}

	col := schemaColumn{
		Name:      r.ColumnName,
		DataType:  dataType(r),
		Nullable:  r.Nullable == "Y",
		Virtual:   r.VirtualColumn == "YES",
		Invisible: r.HiddenColumn == "YES",
	}
	if r.DataDefault.Valid {
		col.Default = r.DataDefault.String
	}
	if d := s.details[tableKey{conID: r.ConID, owner: r.Owner, table: r.TableName}]; d != nil {
		col.Comment = d.ColumnComments[r.ColumnName]
	}
	s.currentTable.Columns = append(s.currentTable.Columns, col)

	s.maybeFlush(false)
}

func (s *schemaCollector) finish() {
	if s.conID != -1 {
		s.maybeFlush(true)
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

// DATA_DEFAULT_VC only exists from 23ai; before that the default is a LONG column that
// cannot be selected safely alongside the rest, so it is left out.
func (c *Check) defaultValueColumn() string {
	major, _, _ := strings.Cut(c.dbVersion, ".")
	if n, err := strconv.Atoi(major); err == nil && n >= 23 {
		return "c.data_default_vc"
	}
	return "CAST(NULL AS VARCHAR2(4000))"
}

type ownerKey struct {
	conID int64
	owner string
}

func (c *Check) schemaOwners(ctx context.Context) (map[ownerKey]string, []string, error) {
	rows, err := c.db.QueryxContext(ctx, schemaOwnersQuery)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query schema owners: %w", err)
	}
	defer rows.Close()

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
		if !schemaOwnerPattern.MatchString(name) {
			log.Warnf("%s skipping schema owner with unexpected characters: %q", c.logPrompt, name)
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
	if len(distinct) > maxSchemaOwners {
		log.Warnf("%s %d schema owners found, collecting only the first %d",
			c.logPrompt, len(distinct), maxSchemaOwners)
		distinct = distinct[:maxSchemaOwners]
	}
	return owners, distinct, nil
}

// A view that is absent (ORA-00942) or shaped differently (ORA-00904) means the property is
// unavailable on this version or not granted; the rest of the collection still stands.
func (c *Check) queryDetails(ctx context.Context, name, template, ownerList string, scan func(*sqlx.Rows) error) {
	rows, err := c.db.QueryxContext(ctx, strings.Replace(template, "/*OWNERS*/", ownerList, 1))
	if err != nil {
		if strings.Contains(err.Error(), "ORA-00942") || strings.Contains(err.Error(), "ORA-00904") {
			log.Debugf("%s table detail %q unavailable: %s", c.logPrompt, name, err)
			return
		}
		log.Warnf("%s failed to collect table detail %q: %s", c.logPrompt, name, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			log.Warnf("%s failed to scan table detail %q: %s", c.logPrompt, name, err)
			return
		}
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

func (c *Check) tableDetails(ctx context.Context, ownerList string) map[tableKey]*tableDetails {
	details := make(map[tableKey]*tableDetails)
	at := func(conID int64, owner, table string) *tableDetails {
		k := tableKey{conID: conID, owner: owner, table: table}
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

	c.queryDetails(ctx, "indexes", indexesQuery, ownerList, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, name, uniqueness, indexType, column string
		if err := rows.Scan(&conID, &owner, &table, &name, &uniqueness, &indexType, &column); err != nil {
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
		idx.Columns = append(idx.Columns, column)
		return nil
	})

	// Referenced tables are resolved after the scan: a foreign key names the constraint it
	// points at, not the table, and that constraint is usually on another table in this set.
	primaryKeys := make(map[string]*constraintInfo)
	c.queryDetails(ctx, "constraints", constraintsQuery, ownerList, func(rows *sqlx.Rows) error {
		var conID int64
		var owner, table, name, ctype, rOwner, rName, column string
		if err := rows.Scan(&conID, &owner, &table, &name, &ctype, &rOwner, &rName, &column); err != nil {
			return err
		}
		d := at(conID, owner, table)
		var con *constraintInfo
		if n := len(d.Constraints); n > 0 && d.Constraints[n-1].Name == name {
			con = d.Constraints[n-1]
		} else {
			con = &constraintInfo{Name: name, Type: constraintType(ctype)}
			if ctype == "R" {
				con.ReferencedOwner = rOwner
				con.referencedKey = fmt.Sprintf("%d|%s|%s", conID, rOwner, rName)
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

func (c *Check) viewDetails(ctx context.Context, ownerList string) map[tableKey]*viewDetails {
	details := make(map[tableKey]*viewDetails)
	at := func(conID int64, owner, name string) *viewDetails {
		k := tableKey{conID: conID, owner: owner, table: name}
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

// ViewCollection emits view metadata as its own kind, mirroring how sqlserver_views is kept
// separate from sqlserver_databases.
func (c *Check) ViewCollection(ctx context.Context, owners map[ownerKey]string, names []string, containers map[int64]string) error {
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}

	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + n + "'"
	}
	ownerList := strings.Join(quoted, ", ")

	query := strings.Replace(viewsQueryTemplate, "/*OWNERS*/", ownerList, 1)
	query = strings.Replace(query, "/*DEFAULT_COL*/", c.defaultValueColumn(), 1)

	rows, err := c.db.QueryxContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query views: %w", err)
	}
	defer rows.Close()

	collector := newViewCollector(c, func(payload []byte) {
		sender.EventPlatformEvent(payload, "dbm-metadata")
	}, c.viewDetails(ctx, ownerList), owners, containers)

	maxViews := c.config.Schemas.MaxViews
	capped := false
	for rows.Next() {
		var r schemaRowDB
		if err := rows.StructScan(&r); err != nil {
			collector.finish()
			return fmt.Errorf("failed to scan view row: %w", err)
		}
		if _, ok := owners[ownerKey{conID: r.ConID, owner: r.Owner}]; !ok {
			continue
		}
		if maxViews > 0 && collector.tablesTotal >= maxViews &&
			(collector.currentView == nil || collector.currentView.Name != r.TableName) {
			capped = true
			break
		}
		collector.addView(r)
	}
	if err := rows.Err(); err != nil {
		collector.finish()
		return fmt.Errorf("failed while streaming view rows: %w", err)
	}
	collector.finish()
	if capped {
		log.Warnf("%s view collection stopped at max_views=%d; some views were not collected",
			c.logPrompt, maxViews)
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

	ctx, cancel := context.WithTimeout(context.Background(), c.config.QueryTimeoutDuration())
	defer cancel()

	owners, names, err := c.schemaOwners(ctx)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		log.Debugf("%s no user schemas to collect", c.logPrompt)
		return nil
	}

	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "'" + n + "'"
	}
	ownerList := strings.Join(quoted, ", ")
	// The cdb_tables and cdb_object_tables branches of the query each carry their own copy of
	// both placeholders, so every occurrence must be substituted, not just the first.
	query := strings.ReplaceAll(schemasQueryTemplate, "/*OWNERS*/", ownerList)
	query = strings.ReplaceAll(query, "/*DEFAULT_COL*/", c.defaultValueColumn())
	details := c.tableDetails(ctx, ownerList)

	containers := c.containerNames(ctx)

	rows, err := c.db.QueryxContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query schemas: %w", err)
	}
	defer rows.Close()

	collector := newSchemaCollector(c, func(payload []byte) {
		sender.EventPlatformEvent(payload, "dbm-metadata")
	}, details, owners, containers)

	for rows.Next() {
		var r schemaRowDB
		if err := rows.StructScan(&r); err != nil {
			collector.finish()
			return fmt.Errorf("failed to scan schema row: %w", err)
		}
		if _, ok := owners[ownerKey{conID: r.ConID, owner: r.Owner}]; !ok {
			continue
		}
		collector.add(r)
	}
	if err := rows.Err(); err != nil {
		collector.finish()
		return fmt.Errorf("failed while streaming schema rows: %w", err)
	}
	collector.finish()

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
