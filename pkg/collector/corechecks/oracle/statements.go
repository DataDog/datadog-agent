package oracle

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/jmoiron/sqlx"
	"golang.org/x/exp/maps"
)

const STATEMENT_METRICS_QUERY = `SELECT 
	c.name as pdb_name,
	%s as query_handle, 
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

type StatementMetricsDB struct {
	PDBName                    string  `db:"PDB_NAME"`
	QueryHandle                string  `db:"QUERY_HANDLE"`
	PlanHashValue              uint64  `db:"PLAN_HASH_VALUE"`
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
	VersionCount               float64 `db:"VERSION_COUNT"`
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
	SharableMem                float64 `db:"SHARABLE_MEM"`
	TypecheckMem               float64 `db:"TYPECHECK_MEM"`
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
			return nil, fmt.Errorf("error preparing statement metrics query: %w", err)
		}
		err = db.Select(&statementMetrics, db.Rebind(query), args...)
		if err != nil {
			return nil, fmt.Errorf("error executing statement metrics query: %w", err)
		}
		return statementMetrics, nil
	}
	return nil, nil
}

func (c *Check) StatementMetrics() error {
	statementMetrics, err := GetStatementsMetricsForKeys(c.db, "force_matching_signature", c.StatementsFilter.ForceMatchingSignatures)
	if err != nil {
		return fmt.Errorf("error collecting statement metrics for force_matching_signature: %w", err)
	}
	statementMetricsAll := statementMetrics
	statementMetrics, err = GetStatementsMetricsForKeys(c.db, "sql_id", c.StatementsFilter.SQLIDs)
	if err != nil {
		return fmt.Errorf("error collecting statement metrics for SQL_IDs: %w", err)
	}
	statementMetricsAll = append(statementMetricsAll, statementMetrics...)
	fmt.Printf("Statements query metrics %+v", &statementMetricsAll)
	return nil
}
