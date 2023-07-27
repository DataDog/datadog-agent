// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	cache "github.com/patrickmn/go-cache"
)

/*
 * We are selecting from sql_fulltext instead of sql_text because sql_text doesn't preserve the new lines.
 * sql_fulltext, despite "full" in its name, truncates the text after the first 1000 characters.
 * For such statements, we will have to get the text from v$sql which has the complete text.
 */
const STATEMENT_METRICS_QUERY = `SELECT /* DD */
	c.name as pdb_name,
	%s,
	plan_hash_value,
	max(dbms_lob.substr(sql_fulltext, 1000, 1)) sql_text,
	length(max(sql_text)) sql_text_length,
	max(sql_id) random_sql_id,
	sum(parse_calls) as parse_calls,
	sum(disk_reads) as disk_reads,
	sum(direct_writes) as direct_writes,
	sum(direct_reads) as direct_reads,
	sum(buffer_gets) as buffer_gets,
	sum(rows_processed) as rows_processed,
	sum(serializable_aborts) as serializable_aborts,
	sum(fetches) as fetches,
	sum(executions) as executions,
	sum(end_of_fetch_count) as end_of_fetch_count,
	sum(loads) as loads,
	sum(version_count) as version_count,
	sum(invalidations) as invalidations,
	sum(px_servers_executions) as px_servers_executions,
	sum(cpu_time) as cpu_time,
	sum(elapsed_time) as elapsed_time,
	sum(application_wait_time) as application_wait_time,
	sum(concurrency_wait_time) as concurrency_wait_time,
	sum(cluster_wait_time) as cluster_wait_time,
	sum(user_io_wait_time) as user_io_wait_time,
	sum(plsql_exec_time) as plsql_exec_time,
	sum(java_exec_time) as java_exec_time,
	sum(sorts) as sorts,
	sum(sharable_mem) as sharable_mem,
	sum(typecheck_mem) as typecheck_mem,
	sum(io_cell_offload_eligible_bytes) as io_cell_offload_eligible_bytes,
	sum(io_interconnect_bytes) as io_interconnect_bytes,
	sum(physical_read_requests) as physical_read_requests,
	sum(physical_read_bytes) as physical_read_bytes,
	sum(physical_write_requests) as physical_write_requests,
	sum(physical_write_bytes) as physical_write_bytes,
	sum(io_cell_uncompressed_bytes) as io_cell_uncompressed_bytes,
	sum(io_cell_offload_returned_bytes) as io_cell_offload_returned_bytes,
	sum(avoided_executions) as avoided_executions
FROM v$sqlstats s, v$containers c
WHERE 
	s.con_id = c.con_id(+)
	AND force_matching_signature %s= 0
GROUP BY c.name, %s, plan_hash_value
HAVING MAX(last_active_time) > sysdate - :seconds/24/60/60
FETCH FIRST :limit ROWS ONLY`

// including sql_id for indexed access
const PLAN_QUERY = `SELECT /* DD */
	timestamp,
	operation,
	options,
	object_name,
	object_type,
	object_alias,
	optimizer,
	id,
	parent_id,
	depth,
	position,
	search_columns,
	cost,
	cardinality,
	bytes,
	partition_start,
	partition_stop,
	other,
	cpu_cost,
	io_cost,
	temp_space,
	access_predicates,
	filter_predicates,
	projection,
	executions,
	last_starts,
	last_output_rows,
	last_cr_buffer_gets,
	last_disk_reads,
	last_disk_writes,
	last_elapsed_time,
	last_memory_used,
	last_degree,
	last_tempseg_size,
	c.name pdb_name
FROM v$sql_plan_statistics_all s, v$containers c
WHERE 
  sql_id = :1 AND plan_hash_value = :2
  AND s.con_id = c.con_id(+)
ORDER BY id, position`

type StatementMetricsKeyDB struct {
	PDBName                string `db:"PDB_NAME"`
	SQLID                  string `db:"SQL_ID"`
	ForceMatchingSignature string `db:"FORCE_MATCHING_SIGNATURE"`
	PlanHashValue          uint64 `db:"PLAN_HASH_VALUE"`
}

type StatementMetricsMonotonicCountDB struct {
	ParseCalls                 float64 `db:"PARSE_CALLS"`
	DiskReads                  float64 `db:"DISK_READS"`
	DirectWrites               float64 `db:"DIRECT_WRITES"`
	DirectReads                float64 `db:"DIRECT_READS"`
	BufferGets                 float64 `db:"BUFFER_GETS"`
	RowsProcessed              float64 `db:"ROWS_PROCESSED"`
	SerializableAborts         float64 `db:"SERIALIZABLE_ABORTS"`
	Fetches                    float64 `db:"FETCHES"`
	Executions                 float64 `db:"EXECUTIONS"`
	EndOfFetchCount            float64 `db:"END_OF_FETCH_COUNT"`
	Loads                      float64 `db:"LOADS"`
	Invalidations              float64 `db:"INVALIDATIONS"`
	PxServersExecutions        float64 `db:"PX_SERVERS_EXECUTIONS"`
	CPUTime                    float64 `db:"CPU_TIME"`
	ElapsedTime                float64 `db:"ELAPSED_TIME"`
	ApplicationWaitTime        float64 `db:"APPLICATION_WAIT_TIME"`
	ConcurrencyWaitTime        float64 `db:"CONCURRENCY_WAIT_TIME"`
	ClusterWaitTime            float64 `db:"CLUSTER_WAIT_TIME"`
	UserIOWaitTime             float64 `db:"USER_IO_WAIT_TIME"`
	PLSQLExecTime              float64 `db:"PLSQL_EXEC_TIME"`
	JavaExecTime               float64 `db:"JAVA_EXEC_TIME"`
	Sorts                      float64 `db:"SORTS"`
	IOCellOffloadEligibleBytes float64 `db:"IO_CELL_OFFLOAD_ELIGIBLE_BYTES"`
	IOCellUncompressedBytes    float64 `db:"IO_CELL_UNCOMPRESSED_BYTES"`
	IOCellOffloadReturnedBytes float64 `db:"IO_CELL_OFFLOAD_RETURNED_BYTES"`
	IOInterconnectBytes        float64 `db:"IO_INTERCONNECT_BYTES"`
	PhysicalReadRequests       float64 `db:"PHYSICAL_READ_REQUESTS"`
	PhysicalReadBytes          float64 `db:"PHYSICAL_READ_BYTES"`
	PhysicalWriteRequests      float64 `db:"PHYSICAL_WRITE_REQUESTS"`
	PhysicalWriteBytes         float64 `db:"PHYSICAL_WRITE_BYTES"`
	ObsoleteCount              float64 `db:"OBSOLETE_COUNT"`
	AvoidedExecutions          float64 `db:"AVOIDED_EXECUTIONS"`
}

type StatementMetricsGaugeDB struct {
	VersionCount float64 `db:"VERSION_COUNT"`
	SharableMem  float64 `db:"SHARABLE_MEM"`
	TypecheckMem float64 `db:"TYPECHECK_MEM"`
}

type StatementMetricsDB struct {
	StatementMetricsKeyDB
	SQLText       string `db:"SQL_TEXT"`
	SQLTextLength int16  `db:"SQL_TEXT_LENGTH"`
	RandomSQLID   string `db:"RANDOM_SQL_ID"`
	StatementMetricsMonotonicCountDB
	StatementMetricsGaugeDB
}

type QueryRow struct {
	QuerySignature string   `json:"query_signature,omitempty" dbm:"query_signature,primary"`
	Tables         []string `json:"dd_tables,omitempty" dbm:"table,tag"`
	Commands       []string `json:"dd_commands,omitempty" dbm:"command,tag"`
	Comments       []string `json:"dd_comments,omitempty" dbm:"comments,tag"`
}

type OracleRowMonotonicCount struct {
	ParseCalls                 float64 `json:"parse_calls,omitempty"`
	DiskReads                  float64 `json:"disk_reads,omitempty"`
	DirectWrites               float64 `json:"direct_writes,omitempty"`
	DirectReads                float64 `json:"direct_reads,omitempty"`
	BufferGets                 float64 `json:"buffer_gets,omitempty"`
	RowsProcessed              float64 `json:"rows_processed,omitempty"`
	SerializableAborts         float64 `json:"serializable_aborts,omitempty"`
	Fetches                    float64 `json:"fetches,omitempty"`
	Executions                 float64 `json:"executions,omitempty"`
	EndOfFetchCount            float64 `json:"end_of_fetch_count,omitempty"`
	Loads                      float64 `json:"loads,omitempty"`
	Invalidations              float64 `json:"invalidations,omitempty"`
	PxServersExecutions        float64 `json:"px_servers_executions,omitempty"`
	CPUTime                    float64 `json:"cpu_time,omitempty"`
	ElapsedTime                float64 `json:"elapsed_time,omitempty"`
	ApplicationWaitTime        float64 `json:"application_wait_time,omitempty"`
	ConcurrencyWaitTime        float64 `json:"concurrency_wait_time,omitempty"`
	ClusterWaitTime            float64 `json:"cluster_wait_time,omitempty"`
	UserIOWaitTime             float64 `json:"user_io_wait_time,omitempty"`
	PLSQLExecTime              float64 `json:"plsql_exec_time,omitempty"`
	JavaExecTime               float64 `json:"java_exec_time,omitempty"`
	Sorts                      float64 `json:"sorts,omitempty"`
	IOCellOffloadEligibleBytes float64 `json:"io_cell_offload_eligible_bytes,omitempty"`
	IOCellUncompressedBytes    float64 `json:"io_cell_uncompressed_bytes,omitempty"`
	IOCellOffloadReturnedBytes float64 `json:"io_cell_offload_returned_bytes,omitempty"`
	IOInterconnectBytes        float64 `json:"io_interconnect_bytes,omitempty"`
	PhysicalReadRequests       float64 `json:"physical_read_requests,omitempty"`
	PhysicalReadBytes          float64 `json:"physical_read_bytes,omitempty"`
	PhysicalWriteRequests      float64 `json:"physical_write_requests,omitempty"`
	PhysicalWriteBytes         float64 `json:"physical_write_bytes,omitempty"`
	ObsoleteCount              float64 `json:"obsolete_count,omitempty"`
	AvoidedExecutions          float64 `json:"avoided_executions,omitempty"`
}

type OracleRowGauge struct {
	VersionCount float64 `json:"version_count,omitempty"`
	SharableMem  float64 `json:"sharable_mem,omitempty"`
	TypecheckMem float64 `json:"typecheck_mem,omitempty"`
}

// OracleRow contains all metrics and tags for a single oracle query
// dbmgen:dbms
type OracleRow struct {
	QueryRow

	// Those are tags that should only have at most a single value per query signature
	SQLText   string `json:"sql_fulltext,omitempty"`
	QueryHash string `json:"query_hash,omitempty"`

	// Secondary dimensions
	PlanHash string `json:"plan_hash,omitempty"`
	PDBName  string `json:"pdb_name,omitempty"`

	OracleRowMonotonicCount
	OracleRowGauge
}

type MetricsPayload struct {
	Host                  string   `json:"host,omitempty"` // Host is the database hostname, not the agent hostname
	Timestamp             float64  `json:"timestamp,omitempty"`
	MinCollectionInterval float64  `json:"min_collection_interval,omitempty"`
	Tags                  []string `json:"tags,omitempty"`
	AgentVersion          string   `json:"ddagentversion,omitempty"`
	AgentHostname         string   `json:"ddagenthostname,omitempty"`

	OracleRows    []OracleRow `json:"oracle_rows,omitempty"`
	OracleVersion string      `json:"oracle_version,omitempty"`
}

type FQTDBMetadata struct {
	Tables   []string `json:"dd_tables"`
	Commands []string `json:"dd_commands"`
}

type FQTDB struct {
	Instance       string        `json:"instance"`
	QuerySignature string        `json:"query_signature"`
	Statement      string        `json:"statement"`
	FQTDBMetadata  FQTDBMetadata `json:"metadata"`
}

type FQTDBOracle struct {
	CDBName string `json:"cdb_name,omitempty"`
}

type FQTPayload struct {
	Timestamp    float64     `json:"timestamp,omitempty"`
	Host         string      `json:"host,omitempty"` // Host is the database hostname, not the agent hostname
	AgentVersion string      `json:"ddagentversion,omitempty"`
	Source       string      `json:"ddsource"`
	Tags         string      `json:"ddtags,omitempty"`
	DBMType      string      `json:"dbm_type"`
	FQTDB        FQTDB       `json:"db"`
	FQTDBOracle  FQTDBOracle `json:"oracle"`
}

type OraclePlan struct {
	PlanHashValue uint64 `json:"plan_hash_value,omitempty"`
	SQLID         string `json:"sql_id,omitempty"`
	Timestamp     string `json:"created,omitempty"`
	OptimizerMode string `json:"optimizer_mode,omitempty"`
	Other         string `json:"other"`
	PDBName       string `json:"pdb_name"`
}

type PlanStatementMetadata struct {
	Tables   []string `json:"tables"`
	Commands []string `json:"commands"`
	Comments []string `json:"comments"`
}

type PlanDefinition struct {
	Operation        string  `json:"operation,omitempty"`
	Options          string  `json:"options,omitempty"`
	ObjectOwner      string  `json:"object_owner,omitempty"`
	ObjectName       string  `json:"object_name,omitempty"`
	ObjectAlias      string  `json:"object_alias,omitempty"`
	ObjectType       string  `json:"object_type,omitempty"`
	PlanStepId       int64   `json:"id,omitempty"`
	ParentId         int64   `json:"parent_id,omitempty"`
	Depth            int64   `json:"depth,omitempty"`
	Position         int64   `json:"position,omitempty"`
	SearchColumns    int64   `json:"search_columns,omitempty"`
	Cost             float64 `json:"cost,omitempty"`
	Cardinality      float64 `json:"cardinality,omitempty"`
	Bytes            float64 `json:"bytes,omitempty"`
	PartitionStart   string  `json:"partition_start,omitempty"`
	PartitionStop    string  `json:"partition_stop,omitempty"`
	CPUCost          float64 `json:"cpu_cost,omitempty"`
	IOCost           float64 `json:"io_cost,omitempty"`
	TempSpace        float64 `json:"temp_space,omitempty"`
	AccessPredicates string  `json:"access_predicates,omitempty"`
	FilterPredicates string  `json:"filter_predicates,omitempty"`
	Projection       string  `json:"projection,omitempty"`
	LastStarts       uint64  `json:"actual_starts,omitempty"`
	LastOutputRows   uint64  `json:"actual_rows,omitempty"`
	LastCRBufferGets uint64  `json:"actual_cr_buffer_gets,omitempty"`
	LastDiskReads    uint64  `json:"actual_disk_reads,omitempty"`
	LastDiskWrites   uint64  `json:"actual_disk_writes,omitempty"`
	LastElapsedTime  uint64  `json:"actual_elapsed_time,omitempty"`
	LastMemoryUsed   uint64  `json:"actual_memory_used,omitempty"`
	LastDegree       uint64  `json:"actual_parallel_degree,omitempty"`
	LastTempsegSize  uint64  `json:"actual_tempseg_size,omitempty"`
}

type PlanPlanDB struct {
	Definition []PlanDefinition `json:"definition"`
	Signature  string           `json:"signature"`
}

type PlanDB struct {
	Instance       string                `json:"instance,omitempty"`
	Plan           PlanPlanDB            `json:"plan,omitempty"`
	QuerySignature string                `json:"query_signature,omitempty"`
	Statement      string                `json:"statement,omitempty"`
	Metadata       PlanStatementMetadata `json:"metadata,omitempty"`
}

type PlanPayload struct {
	Timestamp    float64    `json:"timestamp,omitempty"`
	Host         string     `json:"host,omitempty"` // Host is the database hostname, not the agent hostname
	AgentVersion string     `json:"ddagentversion,omitempty"`
	Source       string     `json:"ddsource"`
	Tags         string     `json:"ddtags,omitempty"`
	DBMType      string     `json:"dbm_type"`
	PlanDB       PlanDB     `json:"db"`
	OraclePlan   OraclePlan `json:"oracle"`
}

type PlanGlobalRow struct {
	SQLID         string         `db:"SQL_ID"`
	ChildNumber   sql.NullInt64  `db:"CHILD_NUMBER"`
	PlanCreated   sql.NullString `db:"TIMESTAMP"`
	OptimizerMode sql.NullString `db:"OPTIMIZER"`
	Other         sql.NullString `db:"OTHER"`
	Executions    sql.NullString `db:"EXECUTIONS"`
	PDBName       sql.NullString `db:"PDB_NAME"`
}
type PlanStepRows struct {
	Operation        sql.NullString  `db:"OPERATION"`
	Options          sql.NullString  `db:"OPTIONS"`
	ObjectOwner      sql.NullString  `db:"OBJECT_OWNER"`
	ObjectName       sql.NullString  `db:"OBJECT_NAME"`
	ObjectAlias      sql.NullString  `db:"OBJECT_ALIAS"`
	ObjectType       sql.NullString  `db:"OBJECT_TYPE"`
	PlanStepId       sql.NullInt64   `db:"ID"`
	ParentId         sql.NullInt64   `db:"PARENT_ID"`
	Depth            sql.NullInt64   `db:"DEPTH"`
	Position         sql.NullInt64   `db:"POSITION"`
	SearchColumns    sql.NullInt64   `db:"SEARCH_COLUMNS"`
	Cost             sql.NullFloat64 `db:"COST"`
	Cardinality      sql.NullFloat64 `db:"CARDINALITY"`
	Bytes            sql.NullFloat64 `db:"BYTES"`
	PartitionStart   sql.NullString  `db:"PARTITION_START"`
	PartitionStop    sql.NullString  `db:"PARTITION_STOP"`
	CPUCost          sql.NullFloat64 `db:"CPU_COST"`
	IOCost           sql.NullFloat64 `db:"IO_COST"`
	TempSpace        sql.NullFloat64 `db:"TEMP_SPACE"`
	AccessPredicates sql.NullString  `db:"ACCESS_PREDICATES"`
	FilterPredicates sql.NullString  `db:"FILTER_PREDICATES"`
	Projection       sql.NullString  `db:"PROJECTION"`
	LastStarts       *uint64         `db:"LAST_STARTS"`
	LastOutputRows   *uint64         `db:"LAST_OUTPUT_ROWS"`
	LastCRBufferGets *uint64         `db:"LAST_CR_BUFFER_GETS"`
	LastDiskReads    *uint64         `db:"LAST_DISK_READS"`
	LastDiskWrites   *uint64         `db:"LAST_DISK_WRITES"`
	LastElapsedTime  *uint64         `db:"LAST_ELAPSED_TIME"`
	LastMemoryUsed   *uint64         `db:"LAST_MEMORY_USED"`
	LastDegree       *uint64         `db:"LAST_DEGREE"`
	LastTempsegSize  *uint64         `db:"LAST_TEMPSEG_SIZE"`
}
type PlanRows struct {
	PlanGlobalRow
	PlanStepRows
}

func GetStatementsMetricsForKeys(c *Check, key string, negator string) ([]StatementMetricsDB, error) {
	var statementMetrics []StatementMetricsDB
	err := selectWrapper(c, &statementMetrics, fmt.Sprintf(STATEMENT_METRICS_QUERY, key, negator, key), 2*c.config.QueryMetrics.CollectionInterval, c.config.QueryMetrics.DBRowsLimit)
	if err != nil {
		return nil, fmt.Errorf("error executing statement metrics query: %w", err)
	}
	return statementMetrics, nil
}

func (c *Check) copyToPreviousMap(newMap map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB) {
	c.statementMetricsMonotonicCountsPrevious = make(map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB)
	for k, v := range newMap {
		c.statementMetricsMonotonicCountsPrevious[k] = v
	}
}

func (c *Check) StatementMetrics() (int, error) {
	start := time.Now()

	if !c.statementsLastRun.IsZero() && start.Sub(c.statementsLastRun).Milliseconds() < c.config.QueryMetrics.CollectionInterval*1000 {
		return 0, nil
	}

	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("GetSender statements metrics")
		return 0, err
	}

	SQLCount := 0
	var oracleRows []OracleRow
	var planErrors uint16
	if c.config.QueryMetrics.Enabled {
		statementMetrics, err := GetStatementsMetricsForKeys(c, "force_matching_signature", "!")
		if err != nil {
			return 0, fmt.Errorf("error collecting statement metrics for force_matching_signature: %w", err)
		}
		log.Tracef("number of collected metrics with force_matching_signature %+v", len(statementMetrics))
		statementMetricsAll := statementMetrics
		statementMetrics, err = GetStatementsMetricsForKeys(c, "sql_id", "")
		if err != nil {
			return 0, fmt.Errorf("error collecting statement metrics for SQL_IDs: %w", err)
		}
		statementMetricsAll = append(statementMetricsAll, statementMetrics...)
		SQLCount = len(statementMetricsAll)
		sender.Count("dd.oracle.statements_metrics.sql_count", float64(SQLCount), "", c.tags)

		// query metrics cache
		newCache := make(map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB)
		if c.statementMetricsMonotonicCountsPrevious == nil {
			c.copyToPreviousMap(newCache)
			return 0, nil
		}

		o := obfuscate.NewObfuscator(obfuscate.Config{SQL: c.config.ObfuscatorOptions})
		defer o.Stop()
		var diff OracleRowMonotonicCount
		planErrors = 0
		for _, statementMetricRow := range statementMetricsAll {
			newCache[statementMetricRow.StatementMetricsKeyDB] = statementMetricRow.StatementMetricsMonotonicCountDB
			previousMonotonic, exists := c.statementMetricsMonotonicCountsPrevious[statementMetricRow.StatementMetricsKeyDB]
			if exists {
				diff = OracleRowMonotonicCount{}
				if diff.ParseCalls = statementMetricRow.ParseCalls - previousMonotonic.ParseCalls; diff.ParseCalls < 0 {
					continue
				}
				if diff.DiskReads = statementMetricRow.DiskReads - previousMonotonic.DiskReads; diff.DiskReads < 0 {
					continue
				}
				if diff.DirectWrites = statementMetricRow.DirectWrites - previousMonotonic.DirectWrites; diff.DirectWrites < 0 {
					continue
				}
				if diff.DirectReads = statementMetricRow.DirectReads - previousMonotonic.DirectReads; diff.DirectReads < 0 {
					continue
				}
				if diff.BufferGets = statementMetricRow.BufferGets - previousMonotonic.BufferGets; diff.BufferGets < 0 {
					continue
				}
				if diff.RowsProcessed = statementMetricRow.RowsProcessed - previousMonotonic.RowsProcessed; diff.RowsProcessed < 0 {
					continue
				}
				if diff.SerializableAborts = statementMetricRow.SerializableAborts - previousMonotonic.SerializableAborts; diff.SerializableAborts < 0 {
					continue
				}
				if diff.Fetches = statementMetricRow.Fetches - previousMonotonic.Fetches; diff.Fetches < 0 {
					continue
				}
				if diff.Executions = statementMetricRow.Executions - previousMonotonic.Executions; diff.Executions <= 0 {
					continue
				}
				if diff.EndOfFetchCount = statementMetricRow.EndOfFetchCount - previousMonotonic.EndOfFetchCount; diff.EndOfFetchCount < 0 {
					continue
				}
				if diff.Loads = statementMetricRow.Loads - previousMonotonic.Loads; diff.Loads < 0 {
					continue
				}
				if diff.Invalidations = statementMetricRow.Invalidations - previousMonotonic.Invalidations; diff.Invalidations < 0 {
					continue
				}
				if diff.PxServersExecutions = statementMetricRow.PxServersExecutions - previousMonotonic.PxServersExecutions; diff.PxServersExecutions < 0 {
					continue
				}
				if diff.CPUTime = statementMetricRow.CPUTime - previousMonotonic.CPUTime; diff.CPUTime < 0 {
					continue
				}
				if diff.ElapsedTime = statementMetricRow.ElapsedTime - previousMonotonic.ElapsedTime; diff.ElapsedTime < 0 {
					continue
				}
				if diff.ApplicationWaitTime = statementMetricRow.ApplicationWaitTime - previousMonotonic.ApplicationWaitTime; diff.ApplicationWaitTime < 0 {
					continue
				}
				if diff.ConcurrencyWaitTime = statementMetricRow.ConcurrencyWaitTime - previousMonotonic.ConcurrencyWaitTime; diff.ConcurrencyWaitTime < 0 {
					continue
				}
				if diff.ClusterWaitTime = statementMetricRow.ClusterWaitTime - previousMonotonic.ClusterWaitTime; diff.ClusterWaitTime < 0 {
					continue
				}
				if diff.UserIOWaitTime = statementMetricRow.UserIOWaitTime - previousMonotonic.UserIOWaitTime; diff.UserIOWaitTime < 0 {
					continue
				}
				if diff.PLSQLExecTime = statementMetricRow.PLSQLExecTime - previousMonotonic.PLSQLExecTime; diff.PLSQLExecTime < 0 {
					continue
				}
				if diff.JavaExecTime = statementMetricRow.JavaExecTime - previousMonotonic.JavaExecTime; diff.JavaExecTime < 0 {
					continue
				}
				if diff.Sorts = statementMetricRow.Sorts - previousMonotonic.Sorts; diff.Sorts < 0 {
					continue
				}
				if diff.IOCellOffloadEligibleBytes = statementMetricRow.IOCellOffloadEligibleBytes - previousMonotonic.IOCellOffloadEligibleBytes; diff.IOCellOffloadEligibleBytes < 0 {
					continue
				}
				if diff.IOCellUncompressedBytes = statementMetricRow.IOCellUncompressedBytes - previousMonotonic.IOCellUncompressedBytes; diff.IOCellUncompressedBytes < 0 {
					continue
				}
				if diff.IOCellOffloadReturnedBytes = statementMetricRow.IOCellOffloadReturnedBytes - previousMonotonic.IOCellOffloadReturnedBytes; diff.IOCellOffloadReturnedBytes < 0 {
					continue
				}
				if diff.IOInterconnectBytes = statementMetricRow.IOInterconnectBytes - previousMonotonic.IOInterconnectBytes; diff.IOInterconnectBytes < 0 {
					continue
				}
				if diff.PhysicalReadRequests = statementMetricRow.PhysicalReadRequests - previousMonotonic.PhysicalReadRequests; diff.PhysicalReadRequests < 0 {
					continue
				}
				if diff.PhysicalReadBytes = statementMetricRow.PhysicalReadBytes - previousMonotonic.PhysicalReadBytes; diff.PhysicalReadBytes < 0 {
					continue
				}
				if diff.PhysicalWriteRequests = statementMetricRow.PhysicalWriteRequests - previousMonotonic.PhysicalWriteRequests; diff.PhysicalWriteRequests < 0 {
					continue
				}
				if diff.PhysicalWriteBytes = statementMetricRow.PhysicalWriteBytes - previousMonotonic.PhysicalWriteBytes; diff.PhysicalWriteBytes < 0 {
					continue
				}
				if diff.ObsoleteCount = statementMetricRow.ObsoleteCount - previousMonotonic.ObsoleteCount; diff.ObsoleteCount < 0 {
					continue
				}
				if diff.AvoidedExecutions = statementMetricRow.AvoidedExecutions - previousMonotonic.AvoidedExecutions; diff.AvoidedExecutions < 0 {
					continue
				}
			} else {
				continue
			}

			queryRow := QueryRow{}
			var SQLStatement string

			if statementMetricRow.SQLTextLength == 1000 {
				err := getFullSQLText(c, &SQLStatement, "sql_id", statementMetricRow.RandomSQLID)
				if err != nil {
					log.Errorf("failed to get the full text %s for sql_id %s", err, statementMetricRow.RandomSQLID)
				}
				if SQLStatement == "" && statementMetricRow.ForceMatchingSignature != "" {
					err := getFullSQLText(c, &SQLStatement, "force_matching_signature", statementMetricRow.ForceMatchingSignature)
					if err != nil {
						log.Errorf("failed to get the full text %s for force_matching_signature %s", err, statementMetricRow.ForceMatchingSignature)
					}
				}
				if SQLStatement != "" {
					statementMetricRow.SQLText = SQLStatement
				}
			}

			obfuscatedStatement, err := c.GetObfuscatedStatement(o, statementMetricRow.SQLText)
			SQLStatement = obfuscatedStatement.Statement
			if err == nil {
				queryRow.QuerySignature = obfuscatedStatement.QuerySignature
				queryRow.Commands = obfuscatedStatement.Commands
				queryRow.Tables = obfuscatedStatement.Tables
			}

			var queryHash string
			if statementMetricRow.StatementMetricsKeyDB.ForceMatchingSignature == "" {
				queryHash = statementMetricRow.StatementMetricsKeyDB.SQLID
			} else {
				queryHash = statementMetricRow.StatementMetricsKeyDB.ForceMatchingSignature
			}
			oracleRow := OracleRow{
				QueryRow:                queryRow,
				SQLText:                 SQLStatement,
				QueryHash:               queryHash,
				PlanHash:                strconv.FormatUint(statementMetricRow.PlanHashValue, 10),
				PDBName:                 c.getFullPDBName(statementMetricRow.PDBName),
				OracleRowMonotonicCount: diff,
				OracleRowGauge:          OracleRowGauge(statementMetricRow.StatementMetricsGaugeDB),
			}

			oracleRows = append(oracleRows, oracleRow)

			if c.fqtEmitted != nil {
				if _, found := c.fqtEmitted.Get(queryRow.QuerySignature); !found {
					FQTDBMetadata := FQTDBMetadata{Tables: queryRow.Tables, Commands: queryRow.Commands}
					FQTDB := FQTDB{Instance: c.cdbName, QuerySignature: queryRow.QuerySignature, Statement: SQLStatement, FQTDBMetadata: FQTDBMetadata}
					FQTDBOracle := FQTDBOracle{
						CDBName: c.cdbName,
					}
					FQTPayload := FQTPayload{
						Timestamp:    float64(time.Now().UnixMilli()),
						Host:         c.dbHostname,
						AgentVersion: c.agentVersion,
						Source:       common.IntegrationName,
						Tags:         c.tagsString,
						DBMType:      "fqt",
						FQTDB:        FQTDB,
						FQTDBOracle:  FQTDBOracle,
					}
					FQTPayloadBytes, err := json.Marshal(FQTPayload)
					if err != nil {
						log.Errorf("Error marshalling fqt payload: %s", err)
					}
					log.Tracef("Query metrics fqt payload %s", string(FQTPayloadBytes))
					sender.EventPlatformEvent(FQTPayloadBytes, "dbm-samples")
					c.fqtEmitted.Set(queryRow.QuerySignature, "1", cache.DefaultExpiration)
				}
			} else {
				log.Error("Internal error: fqtEmitted = nil. The check might have been restarted. Ignore if it doesn't appear anymore.")
			}

			if c.config.ExecutionPlans.Enabled {
				planCacheKey := strconv.FormatUint(statementMetricRow.PlanHashValue, 10)
				_, found := c.planEmitted.Get(planCacheKey)
				if c.config.QueryMetrics.PlanCacheRetention == 0 || !found {
					var planStepsPayload []PlanDefinition
					var planStepsDB []PlanRows
					var oraclePlan OraclePlan
					err = selectWrapper(c, &planStepsDB, PLAN_QUERY, statementMetricRow.RandomSQLID, statementMetricRow.PlanHashValue)

					if err == nil {
						for _, stepRow := range planStepsDB {
							var stepPayload PlanDefinition
							if stepRow.Operation.Valid {
								stepPayload.Operation = stepRow.Operation.String
							}
							if stepRow.Options.Valid {
								stepPayload.Options = stepRow.Options.String
							}
							if stepRow.ObjectOwner.Valid {
								stepPayload.ObjectOwner = stepRow.ObjectOwner.String
							}
							if stepRow.ObjectName.Valid {
								stepPayload.ObjectName = stepRow.ObjectName.String
							}
							if stepRow.ObjectAlias.Valid {
								stepPayload.ObjectAlias = stepRow.ObjectAlias.String
							}
							if stepRow.ObjectType.Valid {
								stepPayload.ObjectType = stepRow.ObjectType.String
							}
							if stepRow.PlanStepId.Valid {
								stepPayload.PlanStepId = stepRow.PlanStepId.Int64
							}
							if stepRow.ParentId.Valid {
								stepPayload.ParentId = stepRow.ParentId.Int64
							}
							if stepRow.Depth.Valid {
								stepPayload.Depth = stepRow.Depth.Int64
							}
							if stepRow.Position.Valid {
								stepPayload.Position = stepRow.Position.Int64
							}
							if stepRow.SearchColumns.Valid {
								stepPayload.SearchColumns = stepRow.SearchColumns.Int64
							}
							if stepRow.Cost.Valid {
								stepPayload.Cost = stepRow.Cost.Float64
							}
							if stepRow.Cardinality.Valid {
								stepPayload.Cardinality = stepRow.Cardinality.Float64
							}
							if stepRow.Bytes.Valid {
								stepPayload.Bytes = stepRow.Bytes.Float64
							}
							if stepRow.PartitionStart.Valid {
								stepPayload.PartitionStart = stepRow.PartitionStart.String
							}
							if stepRow.PartitionStop.Valid {
								stepPayload.PartitionStop = stepRow.PartitionStop.String
							}
							if stepRow.CPUCost.Valid {
								stepPayload.CPUCost = stepRow.CPUCost.Float64
							}
							if stepRow.IOCost.Valid {
								stepPayload.IOCost = stepRow.IOCost.Float64
							}
							if stepRow.TempSpace.Valid {
								stepPayload.TempSpace = stepRow.TempSpace.Float64
							}
							if stepRow.AccessPredicates.Valid {
								obfuscated, err := o.ObfuscateSQLString(stepRow.AccessPredicates.String)
								if err == nil {
									stepPayload.AccessPredicates = obfuscated.Query
								} else {
									stepPayload.AccessPredicates = "obfuscation error"
									log.Errorf("Access obfuscation error")
								}
							}
							if stepRow.FilterPredicates.Valid {
								obfuscated, err := o.ObfuscateSQLString(stepRow.FilterPredicates.String)
								if err == nil {
									stepPayload.FilterPredicates = obfuscated.Query
								} else {
									stepPayload.FilterPredicates = "obfuscation error"
									log.Errorf("Filter obfuscation error")
								}
							}
							if stepRow.Projection.Valid {
								stepPayload.Projection = stepRow.Projection.String
							}
							if stepRow.LastStarts != nil {
								stepPayload.LastStarts = *stepRow.LastStarts
							}
							if stepRow.LastOutputRows != nil {
								stepPayload.LastOutputRows = *stepRow.LastOutputRows
							}
							if stepRow.LastCRBufferGets != nil {
								stepPayload.LastCRBufferGets = *stepRow.LastCRBufferGets
							}
							if stepRow.LastDiskReads != nil {
								stepPayload.LastDiskReads = *stepRow.LastDiskReads
							}
							if stepRow.LastDiskWrites != nil {
								stepPayload.LastDiskWrites = *stepRow.LastDiskWrites
							}
							if stepRow.LastElapsedTime != nil {
								stepPayload.LastElapsedTime = *stepRow.LastElapsedTime
							}
							if stepRow.LastMemoryUsed != nil {
								stepPayload.LastMemoryUsed = *stepRow.LastMemoryUsed
							}
							if stepRow.LastDegree != nil {
								stepPayload.LastDegree = *stepRow.LastDegree
							}
							if stepRow.LastTempsegSize != nil {
								stepPayload.LastTempsegSize = *stepRow.LastTempsegSize
							}
							if stepRow.PlanCreated.Valid && stepRow.PlanCreated.String != "" {
								oraclePlan.Timestamp = stepRow.PlanCreated.String
							}
							if stepRow.OptimizerMode.Valid && stepRow.OptimizerMode.String != "" {
								oraclePlan.OptimizerMode = stepRow.OptimizerMode.String
							}
							if stepRow.Other.Valid && stepRow.Other.String != "" {
								oraclePlan.Other = stepRow.Other.String
							}
							if stepRow.PDBName.Valid && stepRow.PDBName.String != "" {
								oraclePlan.PDBName = stepRow.PDBName.String
							}
							oraclePlan.SQLID = stepRow.SQLID

							planStepsPayload = append(planStepsPayload, stepPayload)
						}
						oraclePlan.PlanHashValue = statementMetricRow.PlanHashValue
						planStatementMetadata := PlanStatementMetadata{
							Tables:   queryRow.Tables,
							Commands: queryRow.Commands,
						}
						planPlanDB := PlanPlanDB{
							Definition: planStepsPayload,
							Signature:  strconv.FormatUint(statementMetricRow.PlanHashValue, 10),
						}
						planDB := PlanDB{
							Instance:       c.cdbName,
							Plan:           planPlanDB,
							QuerySignature: queryRow.QuerySignature,
							Statement:      SQLStatement,
							Metadata:       planStatementMetadata,
						}
						planPayload := PlanPayload{
							Timestamp:    float64(time.Now().UnixMilli()),
							Host:         c.dbHostname,
							AgentVersion: c.agentVersion,
							Source:       common.IntegrationName,
							Tags:         strings.Join(c.tags, ","),
							DBMType:      "plan",
							PlanDB:       planDB,
							OraclePlan:   oraclePlan,
						}
						planPayloadBytes, err := json.Marshal(planPayload)
						if err != nil {
							log.Errorf("Error marshalling plan payload: %s", err)
						}

						sender.EventPlatformEvent(planPayloadBytes, "dbm-samples")
						log.Tracef("Plan payload %+v", string(planPayloadBytes))
						c.planEmitted.Set(planCacheKey, "1", cache.DefaultExpiration)
					} else {
						planErrors++
						log.Errorf("failed getting execution plan %s for SQL_ID: %s, plan_hash_value: %d", err, statementMetricRow.RandomSQLID, statementMetricRow.PlanHashValue)
					}
				}
			}
		}

		c.copyToPreviousMap(newCache)
		c.statementsLastRun = start
	} else {
		heartbeatStatement := "__other__"
		queryRowHeartbeat := QueryRow{QuerySignature: heartbeatStatement}

		oracleRow := OracleRow{
			QueryRow:                queryRowHeartbeat,
			SQLText:                 heartbeatStatement,
			QueryHash:               "heartbeatQH",
			PlanHash:                "hearbeatPH",
			PDBName:                 c.getFullPDBName("heartbeatPDB"),
			OracleRowMonotonicCount: OracleRowMonotonicCount{Executions: 1, ElapsedTime: 1},
		}
		oracleRows = append(oracleRows, oracleRow)
	}
	payload := MetricsPayload{
		Host:                  c.dbHostname,
		Timestamp:             float64(time.Now().UnixMilli()),
		MinCollectionInterval: c.checkInterval,
		Tags:                  c.tags,
		AgentVersion:          c.agentVersion,
		OracleRows:            oracleRows,
		OracleVersion:         c.dbVersion,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("Error marshalling query metrics payload: %s", err)
		return 0, err
	}

	log.Tracef("Query metrics payload %s", strings.ReplaceAll(string(payloadBytes), "@", "XX"))

	sender.EventPlatformEvent(payloadBytes, "dbm-metrics")
	sender.Gauge("dd.oracle.statements_metrics.time_ms", float64(time.Since(start).Milliseconds()), "", c.tags)
	if c.config.ExecutionPlans.Enabled {
		sender.Gauge("dd.oracle.plan_errors.count", float64(planErrors), "", c.tags)
	}
	sender.Commit()

	c.statementsFilter.SQLIDs = nil
	c.statementsFilter.ForceMatchingSignatures = nil
	c.statementsCache.SQLIDs = nil
	c.statementsCache.forceMatchingSignatures = nil

	if planErrors > 0 {
		return SQLCount, fmt.Errorf("SQL statements processed: %d, plan errors: %d", SQLCount, planErrors)
	}
	return SQLCount, nil
}

func getFullSQLText(c *Check, SQLStatement *string, key string, value string) error {
	sql := fmt.Sprintf("SELECT /* DD */ sql_fulltext FROM v$sql WHERE %s = :v AND rownum = 1", key)
	err := c.db.Get(SQLStatement, sql, value)
	if err != nil {
		log.Warnf("failed to select full SQL text based on %s: %s \n%s", key, err, sql)
		recoverFromDeadConnection(c, err)
	}
	return err
}

func recoverFromDeadConnection(c *Check, err error) {
	if err != nil && (strings.Contains(err.Error(), "ORA-01012") || strings.Contains(err.Error(), "database is closed")) {
		db, err := c.Connect()
		if err != nil {
			c.Teardown()
			log.Errorf("failed to tear down check: %s", err)
		}
		c.db = db
	}
}
