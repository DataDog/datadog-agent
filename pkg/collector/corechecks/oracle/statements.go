package oracle

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/jmoiron/sqlx"
	"golang.org/x/exp/maps"
)

const STATEMENT_METRICS_QUERY = `SELECT 
	c.name as pdb_name,
	%s,
	plan_hash_value, 
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
	AND %s IN (?)
GROUP BY c.name, %s, plan_hash_value`

type StatementMetricsKeyDB struct {
	PDBName                string `db:"PDB_NAME"`
	SQLID                  string `db:"SQL_ID"`
	ForceMatchingSignature uint64 `db:"FORCE_MATCHING_SIGNATURE"`
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
	StatementMetricsMonotonicCountDB
	StatementMetricsGaugeDB
}

type QueryRow struct {
	QuerySignature string   `json:"query_signature,omitempty" dbm:"query_signature,primary"`
	Tables         []string `json:"dd_tables,omitempty" dbm:"table,tag"`
	Commands       []string `json:"dd_commands,omitempty" dbm:"command,tag"`
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
	EndOfFetchCount            float64 `json:"end_of_fetch_count,omitT"`
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

func ConstructStatementMetricsQueryBlock(sqlHandleColumn string) string {
	return fmt.Sprintf(STATEMENT_METRICS_QUERY, sqlHandleColumn, sqlHandleColumn, sqlHandleColumn)
}

func GetStatementsMetricsForKeys[K comparable](db *sqlx.DB, keyName string, keys map[K]int) ([]StatementMetricsDB, error) {
	if len(keys) != 0 {
		var statementMetrics []StatementMetricsDB
		statements_metrics_query := ConstructStatementMetricsQueryBlock(keyName)
		keysSlice := maps.Keys(keys)
		log.Tracef("Statements query metrics keys %s: %+v", keyName, keysSlice)
		query, args, err := sqlx.In(statements_metrics_query, keysSlice)
		if err != nil {
			return nil, fmt.Errorf("error preparing statement metrics query: %w %s", err, statements_metrics_query)
		}
		err = db.Select(&statementMetrics, db.Rebind(query), args...)
		if err != nil {
			return nil, fmt.Errorf("error executing statement metrics query: %w %s", err, statements_metrics_query)
		}
		return statementMetrics, nil
	}
	return nil, nil
}

func (c *Check) copyToPreviousMap(newMap map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB) {
	c.statementMetricsMonotonicCountsPrevious = make(map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB)
	for k, v := range newMap {
		c.statementMetricsMonotonicCountsPrevious[k] = v
	}
}

func (c *Check) StatementMetrics() error {
	statementMetrics, err := GetStatementsMetricsForKeys(c.db, "force_matching_signature", c.statementsFilter.ForceMatchingSignatures)
	if err != nil {
		return fmt.Errorf("error collecting statement metrics for force_matching_signature: %w", err)
	}
	statementMetricsAll := statementMetrics
	statementMetrics, err = GetStatementsMetricsForKeys(c.db, "sql_id", c.statementsFilter.SQLIDs)
	if err != nil {
		return fmt.Errorf("error collecting statement metrics for SQL_IDs: %w", err)
	}
	statementMetricsAll = append(statementMetricsAll, statementMetrics...)

	newCache := make(map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB)
	if c.statementMetricsMonotonicCountsPrevious == nil {
		c.copyToPreviousMap(newCache)
		return nil
	}
	for _, statementMetricRow := range statementMetricsAll {
		fmt.Printf("statements row %+v \n", statementMetricRow)
		newCache[statementMetricRow.StatementMetricsKeyDB] = statementMetricRow.StatementMetricsMonotonicCountDB
		previousMonotonic, exists := c.statementMetricsMonotonicCountsPrevious[statementMetricRow.StatementMetricsKeyDB]
		if exists {
			diff := StatementMetricsMonotonicCountDB{}
			diff.ParseCalls = statementMetricRow.ParseCalls - previousMonotonic.ParseCalls
			diff.DiskReads = statementMetricRow.DiskReads - previousMonotonic.DiskReads
			diff.DirectWrites = statementMetricRow.DirectWrites - previousMonotonic.DirectWrites
			diff.DirectReads = statementMetricRow.DirectReads - previousMonotonic.DirectReads
			diff.BufferGets = statementMetricRow.BufferGets - previousMonotonic.BufferGets
			diff.RowsProcessed = statementMetricRow.RowsProcessed - previousMonotonic.RowsProcessed
			diff.SerializableAborts = statementMetricRow.SerializableAborts - previousMonotonic.SerializableAborts
			diff.Fetches = statementMetricRow.Fetches - previousMonotonic.Fetches
			diff.Executions = statementMetricRow.Executions - previousMonotonic.Executions
			diff.EndOfFetchCount = statementMetricRow.EndOfFetchCount - previousMonotonic.EndOfFetchCount
			diff.Loads = statementMetricRow.Loads - previousMonotonic.Loads
			diff.Invalidations = statementMetricRow.Invalidations - previousMonotonic.Invalidations
			diff.PxServersExecutions = statementMetricRow.PxServersExecutions - previousMonotonic.PxServersExecutions
			diff.CPUTime = statementMetricRow.CPUTime - previousMonotonic.CPUTime
			diff.ElapsedTime = statementMetricRow.ElapsedTime - previousMonotonic.ElapsedTime
			diff.ApplicationWaitTime = statementMetricRow.ApplicationWaitTime - previousMonotonic.ApplicationWaitTime
			diff.ConcurrencyWaitTime = statementMetricRow.ConcurrencyWaitTime - previousMonotonic.ConcurrencyWaitTime
			diff.ClusterWaitTime = statementMetricRow.ClusterWaitTime - previousMonotonic.ClusterWaitTime
			diff.UserIOWaitTime = statementMetricRow.UserIOWaitTime - previousMonotonic.UserIOWaitTime
			diff.PLSQLExecTime = statementMetricRow.PLSQLExecTime - previousMonotonic.PLSQLExecTime
			diff.JavaExecTime = statementMetricRow.JavaExecTime - previousMonotonic.JavaExecTime
			diff.Sorts = statementMetricRow.Sorts - previousMonotonic.Sorts
			diff.IOCellOffloadEligibleBytes = statementMetricRow.IOCellOffloadEligibleBytes - previousMonotonic.IOCellOffloadEligibleBytes
			diff.IOCellUncompressedBytes = statementMetricRow.IOCellUncompressedBytes - previousMonotonic.IOCellUncompressedBytes
			diff.IOCellOffloadReturnedBytes = statementMetricRow.IOCellOffloadReturnedBytes - previousMonotonic.IOCellOffloadReturnedBytes
			diff.IOInterconnectBytes = statementMetricRow.IOInterconnectBytes - previousMonotonic.IOInterconnectBytes
			diff.PhysicalReadRequests = statementMetricRow.PhysicalReadRequests - previousMonotonic.PhysicalReadRequests
			diff.PhysicalReadBytes = statementMetricRow.PhysicalReadBytes - previousMonotonic.PhysicalReadBytes
			diff.PhysicalWriteRequests = statementMetricRow.PhysicalWriteRequests - previousMonotonic.PhysicalWriteRequests
			diff.PhysicalWriteBytes = statementMetricRow.PhysicalWriteBytes - previousMonotonic.PhysicalWriteBytes
			diff.ObsoleteCount = statementMetricRow.ObsoleteCount - previousMonotonic.ObsoleteCount
			diff.AvoidedExecutions = statementMetricRow.AvoidedExecutions - previousMonotonic.AvoidedExecutions
		} else {
			continue
		}

		var queryHashCol string
		if statementMetricRow.StatementMetricsKeyDB.ForceMatchingSignature == 0 {
			queryHashCol = "sql_id"
		} else {
			queryHashCol = "force_matching_signature"
		}
		p := map[string]interface{}{
			"force_matching_signature": statementMetricRow.StatementMetricsKeyDB.ForceMatchingSignature,
			"sql_id":                   statementMetricRow.StatementMetricsKeyDB.SQLID,
		}
		SQLTextQuery := fmt.Sprintf("SELECT sql_fulltext FROM v$sqlstats WHERE %s=:%s AND rownum = 1", queryHashCol, queryHashCol)
		rows, err := c.db.NamedQuery(SQLTextQuery, p)
		if err != nil {
			log.Errorf("statements error named exec %s ", err)
			continue
		}
		defer rows.Close()
		rows.Next()
		cols, err := rows.SliceScan()
		if err != nil {
			log.Errorf("statements scan error %s ", err)
		}
		fmt.Printf("statements sql %s \n", cols[0])

		/*
			for rows.Next() {
				// cols is an []interface{} of all of the column results
				cols, err := rows.SliceScan()
				fmt.Printf("statements cols %+v \n", cols)

			}
		*/
		/*
			err = row.Scan(&SQLText)
			if err != nil {
				log.Errorf("failed to get text for query %s=%s %s ", queryHashSource, queryHashSource, queryHashFilter, err)
			}
		*/
		//fmt.Printf("statements SQL %s \n", SQLText)
	}
	c.copyToPreviousMap(newCache)

	return nil
}
