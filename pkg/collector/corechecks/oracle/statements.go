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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	cache "github.com/patrickmn/go-cache"
)

//nolint:revive // TODO(DBM) Fix revive linter
type StatementMetricsKeyDB struct {
	ConID                  int    `db:"CON_ID"`
	PDBName                string `db:"PDB_NAME"`
	SQLID                  string `db:"SQL_ID"`
	ForceMatchingSignature string `db:"FORCE_MATCHING_SIGNATURE"`
	PlanHashValue          uint64 `db:"PLAN_HASH_VALUE"`
}

//nolint:revive // TODO(DBM) Fix revive linter
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

//nolint:revive // TODO(DBM) Fix revive linter
type StatementMetricsGaugeDB struct {
	VersionCount float64 `db:"VERSION_COUNT"`
	SharableMem  float64 `db:"SHARABLE_MEM"`
	TypecheckMem float64 `db:"TYPECHECK_MEM"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type StatementMetricsDB struct {
	StatementMetricsKeyDB
	SQLText       string `db:"SQL_TEXT"`
	SQLTextLength int16  `db:"SQL_TEXT_LENGTH"`
	StatementMetricsMonotonicCountDB
	StatementMetricsGaugeDB
}

//nolint:revive // TODO(DBM) Fix revive linter
type QueryRow struct {
	QuerySignature string   `json:"query_signature,omitempty" dbm:"query_signature,primary"`
	Tables         []string `json:"dd_tables,omitempty" dbm:"table,tag"`
	Commands       []string `json:"dd_commands,omitempty" dbm:"command,tag"`
	Comments       []string `json:"dd_comments,omitempty" dbm:"comments,tag"`
}

//nolint:revive // TODO(DBM) Fix revive linter
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

//nolint:revive // TODO(DBM) Fix revive linter
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

//nolint:revive // TODO(DBM) Fix revive linter
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

//nolint:revive // TODO(DBM) Fix revive linter
type FQTDBMetadata struct {
	Tables   []string `json:"dd_tables"`
	Commands []string `json:"dd_commands"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type FQTDB struct {
	Instance       string        `json:"instance"`
	QuerySignature string        `json:"query_signature"`
	Statement      string        `json:"statement"`
	FQTDBMetadata  FQTDBMetadata `json:"metadata"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type FQTDBOracle struct {
	CDBName string `json:"cdb_name,omitempty"`
}

//nolint:revive // TODO(DBM) Fix revive linter
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

//nolint:revive // TODO(DBM) Fix revive linter
type OraclePlan struct {
	PlanHashValue          uint64  `json:"plan_hash_value,omitempty"`
	SQLID                  string  `json:"sql_id,omitempty"`
	Timestamp              string  `json:"created,omitempty"`
	OptimizerMode          string  `json:"optimizer_mode,omitempty"`
	Other                  string  `json:"other"`
	PDBName                string  `json:"pdb_name"`
	Executions             float64 `json:"executions,omitempty"`
	ElapsedTime            float64 `json:"elapsed_time,omitempty"`
	ForceMatchingSignature string  `json:"force_matching_signature,omitempty"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type PlanStatementMetadata struct {
	Tables   []string `json:"tables"`
	Commands []string `json:"commands"`
	Comments []string `json:"comments"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type PlanDefinition struct {
	Operation   string `json:"operation,omitempty"`
	Options     string `json:"options,omitempty"`
	ObjectOwner string `json:"object_owner,omitempty"`
	ObjectName  string `json:"object_name,omitempty"`
	ObjectAlias string `json:"object_alias,omitempty"`
	ObjectType  string `json:"object_type,omitempty"`
	//nolint:revive // TODO(DBM) Fix revive linter
	PlanStepId       int64   `json:"id"`
	ParentId         int64   `json:"parent_id"`
	Depth            int64   `json:"depth"`
	Position         int64   `json:"position"`
	SearchColumns    int64   `json:"search_columns,omitempty"`
	Cost             float64 `json:"cost"`
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

//nolint:revive // TODO(DBM) Fix revive linter
type PlanPlanDB struct {
	Definition []PlanDefinition `json:"definition"`
	Signature  string           `json:"signature"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type PlanDB struct {
	Instance       string                `json:"instance,omitempty"`
	Plan           PlanPlanDB            `json:"plan,omitempty"`
	QuerySignature string                `json:"query_signature,omitempty"`
	Statement      string                `json:"statement,omitempty"`
	Metadata       PlanStatementMetadata `json:"metadata,omitempty"`
}

//nolint:revive // TODO(DBM) Fix revive linter
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

//nolint:revive // TODO(DBM) Fix revive linter
type PlanGlobalRow struct {
	SQLID         string         `db:"SQL_ID"`
	ChildNumber   sql.NullInt64  `db:"CHILD_NUMBER"`
	PlanCreated   sql.NullString `db:"TIMESTAMP"`
	OptimizerMode sql.NullString `db:"OPTIMIZER"`
	Other         sql.NullString `db:"OTHER"`
	Executions    sql.NullString `db:"EXECUTIONS"`
	PDBName       sql.NullString `db:"PDB_NAME"`
}

//nolint:revive // TODO(DBM) Fix revive linter
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

//nolint:revive // TODO(DBM) Fix revive linter
type PlanRows struct {
	PlanGlobalRow
	PlanStepRows
}

func (c *Check) copyToPreviousMap(newMap map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB) {
	c.statementMetricsMonotonicCountsPrevious = make(map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB)
	for k, v := range newMap {
		c.statementMetricsMonotonicCountsPrevious[k] = v
	}
}

func handlePredicate(predicateType string, dbValue sql.NullString, payloadValue *string, statement StatementMetricsDB, c *Check, o *obfuscate.Obfuscator) {
	if dbValue.Valid && dbValue.String != "" {
		obfuscated, err := o.ObfuscateSQLString(dbValue.String)
		if err == nil {
			*payloadValue = obfuscated.Query
		} else {
			*payloadValue = fmt.Sprintf("%s obfuscation error %d", predicateType, len(dbValue.String))
			//*payloadValue = dbValue.String
			logEntry := fmt.Sprintf("%s %s for sql_id: %s, plan_hash_value: %d", c.logPrompt, *payloadValue, statement.SQLID, statement.PlanHashValue)
			if c.config.ExecutionPlans.LogUnobfuscatedPlans {
				logEntry = fmt.Sprintf("%s unobfuscated filter: %s", logEntry, dbValue.String)
			}
			log.Error(logEntry)
		}
	}
}

//nolint:revive // TODO(DBM) Fix revive linter
func (c *Check) StatementMetrics() (int, error) {
	if !checkIntervalExpired(&c.statementsLastRun, c.config.QueryMetrics.CollectionInterval) {
		return 0, nil
	}
	start := c.statementsLastRun

	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("%s GetSender statements metrics %s", c.logPrompt, err)
		return 0, err
	}

	SQLCount := 0
	var oracleRows []OracleRow
	var planErrors uint16
	queries := getStatementMetricsQueries(c)
	if c.config.QueryMetrics.Enabled {
		var statementMetrics []StatementMetricsDB
		var sql string
		if c.config.QueryMetrics.DisableLastActive {
			sql = queries[fmsRandomQuery]
		} else {
			sql = queries[fmsLastActiveQuery]
		}

		var lookback int64
		if c.config.QueryMetrics.Lookback != 0 {
			lookback = c.config.QueryMetrics.Lookback
		} else {
			lookback = 2 * c.config.QueryMetrics.CollectionInterval
		}

		err := selectWrapper(
			c,
			&statementMetrics,
			sql,
			lookback,
			c.config.QueryMetrics.DBRowsLimit,
		)
		if err != nil {
			return 0, fmt.Errorf("error collecting statement metrics for force_matching_signature: %w %s", err, sql)
		}
		log.Debugf("%s number of collected metrics with force_matching_signature %+v", c.logPrompt, len(statementMetrics))

		statementMetricsAll := make([]StatementMetricsDB, len(statementMetrics))
		copy(statementMetricsAll, statementMetrics)

		sql = queries[sqlIDQuery]
		err = selectWrapper(
			c,
			&statementMetrics,
			sql,
			lookback,
			c.config.QueryMetrics.DBRowsLimit,
		)
		if err != nil {
			return 0, fmt.Errorf("error collecting statement metrics for SQL_IDs: %w %s", err, sql)
		}
		log.Debugf("%s number of collected metrics with SQL_ID %+v", c.logPrompt, len(statementMetrics))
		statementMetricsAll = append(statementMetricsAll, statementMetrics...)
		SQLCount = len(statementMetricsAll)

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
		sendPlan := true
		for i, statementMetricRow := range statementMetricsAll {
			var trace bool
			for _, t := range c.config.QueryMetrics.Trackers {
				if len(t.ContainsText) > 0 {
					for _, q := range t.ContainsText {
						if strings.Contains(statementMetricRow.SQLText, q) {
							trace = true
						} else {
							trace = false
							break
						}
					}
					if trace {
						break
					}
				}
			}
			if trace {
				log.Infof("%s qm_tracker queried: %+v", c.logPrompt, statementMetricRow)
			}

			newCache[statementMetricRow.StatementMetricsKeyDB] = statementMetricRow.StatementMetricsMonotonicCountDB
			previousMonotonic, exists := c.statementMetricsMonotonicCountsPrevious[statementMetricRow.StatementMetricsKeyDB]
			if exists {
				if trace {
					log.Infof("%s qm_tracker previous: %+v %+v", c.logPrompt, statementMetricRow.StatementMetricsKeyDB, previousMonotonic)
				}
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
				if diff.Executions = statementMetricRow.Executions - previousMonotonic.Executions; diff.Executions < 0 {
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

			if statementMetricRow.SQLTextLength == MaxSQLFullTextVSQLStats {
				err := getFullSQLText(c, &SQLStatement, "sql_id", statementMetricRow.SQLID)
				if err != nil {
					log.Errorf("%s failed to get the full text %s for sql_id %s", c.logPrompt, err, statementMetricRow.SQLID)
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
			if trace {
				log.Infof("%s qm_tracker payload: %+v", c.logPrompt, oracleRow)
			}

			if c.fqtEmitted == nil {
				c.fqtEmitted = getFqtEmittedCache()
			}

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
					log.Errorf("%s Error marshalling fqt payload: %s", c.logPrompt, err)
				}
				log.Debugf("%s Query metrics fqt payload %s", c.logPrompt, string(FQTPayloadBytes))
				sender.EventPlatformEvent(FQTPayloadBytes, "dbm-samples")
				c.fqtEmitted.Set(queryRow.QuerySignature, "1", cache.DefaultExpiration)
			}

			if c.config.ExecutionPlans.Enabled && sendPlan {
				if (i+1)%10 == 0 && time.Since(start).Seconds() >= float64(c.config.QueryMetrics.MaxRunTime) {
					sendPlan = false
				}

				planCacheKey := strconv.FormatUint(statementMetricRow.PlanHashValue, 10)
				if c.planEmitted == nil {
					c.planEmitted = getPlanEmittedCache(c)
				}
				_, found := c.planEmitted.Get(planCacheKey)
				if c.config.ExecutionPlans.PlanCacheRetention == 0 || !found {
					var planStepsPayload []PlanDefinition
					var planStepsDB []PlanRows
					var oraclePlan OraclePlan

					var planQuery string
					if isDbVersionGreaterOrEqualThan(c, minMultitenantVersion) {
						planQuery = planQuery12
						err = selectWrapper(c, &planStepsDB, planQuery, statementMetricRow.SQLID, statementMetricRow.PlanHashValue, statementMetricRow.ConID)
					} else {
						planQuery = planQuery11
						err = selectWrapper(c, &planStepsDB, planQuery, statementMetricRow.SQLID, statementMetricRow.PlanHashValue)
					}

					if err == nil {
						if len(planStepsDB) > 0 {
							var firstChildNumber int64
							for i, stepRow := range planStepsDB {
								if !stepRow.ChildNumber.Valid {
									log.Errorf("%s invalid child numner in execution plan", c.logPrompt)
									break
								}
								if i == 0 {
									firstChildNumber = stepRow.ChildNumber.Int64
								} else {
									if firstChildNumber != stepRow.ChildNumber.Int64 {
										break
									}
								}
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
								handlePredicate("access", stepRow.AccessPredicates, &stepPayload.AccessPredicates, statementMetricRow, c, o)
								handlePredicate("filter", stepRow.FilterPredicates, &stepPayload.FilterPredicates, statementMetricRow, c, o)
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

								oraclePlan.PDBName = statementMetricRow.PDBName
								oraclePlan.SQLID = statementMetricRow.SQLID
								oraclePlan.ForceMatchingSignature = statementMetricRow.ForceMatchingSignature
								oraclePlan.Executions = statementMetricRow.Executions
								oraclePlan.ElapsedTime = statementMetricRow.ElapsedTime

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
							tags := strings.Join(append(c.tags, fmt.Sprintf("pdb:%s", statementMetricRow.PDBName)), ",")

							planPayload := PlanPayload{
								Timestamp:    float64(time.Now().UnixMilli()),
								Host:         c.dbHostname,
								AgentVersion: c.agentVersion,
								Source:       common.IntegrationName,
								Tags:         tags,
								DBMType:      "plan",
								PlanDB:       planDB,
								OraclePlan:   oraclePlan,
							}
							planPayloadBytes, err := json.Marshal(planPayload)
							if err != nil {
								log.Errorf("%s Error marshalling plan payload: %s", c.logPrompt, err)
							}

							sender.EventPlatformEvent(planPayloadBytes, "dbm-samples")
							log.Debugf("%s Plan payload %+v", c.logPrompt, string(planPayloadBytes))
							c.planEmitted.Set(planCacheKey, "1", cache.DefaultExpiration)
						} else {
							log.Infof("%s Plan for SQL_ID %s and plan_hash_value: %d not found", c.logPrompt, statementMetricRow.SQLID, statementMetricRow.PlanHashValue)
						}
					} else {
						planErrors++
						log.Errorf("%s failed getting execution plan %s for SQL_ID: %s, plan_hash_value: %d", c.logPrompt, err, statementMetricRow.SQLID, statementMetricRow.PlanHashValue)
					}
				}

			}
		}
		c.copyToPreviousMap(newCache)
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

	c.lastOracleRows = make([]OracleRow, len(oracleRows))
	copy(c.lastOracleRows, oracleRows)

	payload := MetricsPayload{
		Host:                  c.dbHostname,
		Timestamp:             float64(time.Now().UnixMilli()),
		MinCollectionInterval: c.checkInterval,
		Tags:                  c.tags,
		AgentVersion:          c.agentVersion,
		AgentHostname:         c.agentHostname,
		OracleRows:            oracleRows,
		OracleVersion:         c.dbVersion,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("%s Error marshalling query metrics payload: %s", c.logPrompt, err)
		return 0, err
	}

	log.Debugf("%s Query metrics payload %s", c.logPrompt, strings.ReplaceAll(string(payloadBytes), "@", "XX"))

	sender.EventPlatformEvent(payloadBytes, "dbm-metrics")
	sendMetricWithDefaultTags(c, gauge, "dd.oracle.statements_metrics.time_ms", float64(time.Since(start).Milliseconds()))
	if c.config.ExecutionPlans.Enabled {
		sendMetricWithDefaultTags(c, gauge, "dd.oracle.plan_errors.count", float64(planErrors))
	}
	sender.Commit()

	if planErrors > 0 {
		return SQLCount, fmt.Errorf("SQL statements processed: %d, plan errors: %d", SQLCount, planErrors)
	}

	return SQLCount, nil
}
