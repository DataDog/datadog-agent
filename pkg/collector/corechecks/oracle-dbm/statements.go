// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
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
	AND %s IN (%s) %s
GROUP BY c.name, %s, plan_hash_value`

type StatementMetricsKeyDB struct {
	PDBName string `db:"PDB_NAME"`
	SQLID   string `db:"SQL_ID"`
	//ForceMatchingSignature uint64 `db:"FORCE_MATCHING_SIGNATURE"`
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

func ConstructStatementMetricsQueryBlock(sqlHandleColumn string, whereClause string, bindPlaceholder string) string {
	return fmt.Sprintf(STATEMENT_METRICS_QUERY, sqlHandleColumn, sqlHandleColumn, bindPlaceholder, whereClause, sqlHandleColumn)
}

// func GetStatementsMetricsForKeys[K comparable](db *sqlx.DB, keyName string, whereClause string, keys map[K]int) ([]StatementMetricsDB, error) {
func GetStatementsMetricsForKeys[K comparable](c *Check, keyName string, whereClause string, keys map[K]int) ([]StatementMetricsDB, error) {
	if len(keys) != 0 {
		db := c.db
		driver := c.driver
		var bindPlaceholder string
		keysSlice := maps.Keys(keys)

		if driver == common.Godror {
			bindPlaceholder = "?"
		} else if driver == common.GoOra {
			// workaround for https://github.com/jmoiron/sqlx/issues/854
			for i := range keysSlice {
				if i > 0 {
					bindPlaceholder = bindPlaceholder + ","
				}
				bindPlaceholder = fmt.Sprintf("%s:%d", bindPlaceholder, i+1)
			}
		} else {
			return nil, fmt.Errorf("statements wrong driver %s", driver)
		}

		var statementMetrics []StatementMetricsDB
		statements_metrics_query := ConstructStatementMetricsQueryBlock(keyName, whereClause, bindPlaceholder)

		log.Tracef("Statements query metrics keys %s: %+v", keyName, keysSlice)

		if driver == common.Godror {
			query, args, err := sqlx.In(statements_metrics_query, keysSlice)
			if err != nil {
				return nil, fmt.Errorf("error preparing statement metrics query: %w %s", err, statements_metrics_query)
			}
			err = db.Select(&statementMetrics, db.Rebind(query), args...)
			if err != nil {
				return nil, fmt.Errorf("error executing statement metrics query: %w %s", err, statements_metrics_query)
			}
		} else if driver == common.GoOra {
			// workaround for https://github.com/jmoiron/sqlx/issues/854
			convertedSlice := make([]any, len(keysSlice))
			for i := range keysSlice {
				convertedSlice[i] = K(keysSlice[i])
			}
			err := db.Select(&statementMetrics, statements_metrics_query, convertedSlice...)
			if err != nil {
				return nil, fmt.Errorf("error executing statement metrics query: %w %s", err, statements_metrics_query)
			}
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

func (c *Check) StatementMetrics() (int, error) {
	start := time.Now()
	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("GetSender statements metrics")
		return 0, err
	}

	SQLTextErrors := 0
	SQLCount := 0
	var oracleRows []OracleRow
	if c.config.QueryMetrics {
		statementMetrics, err := GetStatementsMetricsForKeys(c, "force_matching_signature", "AND force_matching_signature != 0", c.statementsFilter.ForceMatchingSignatures)
		if err != nil {
			return 0, fmt.Errorf("error collecting statement metrics for force_matching_signature: %w", err)
		}
		log.Tracef("number of collected metrics with force_matching_signature %+v", len(statementMetrics))
		statementMetricsAll := statementMetrics
		statementMetrics, err = GetStatementsMetricsForKeys(c, "sql_id", " ", c.statementsFilter.SQLIDs)
		if err != nil {
			return 0, fmt.Errorf("error collecting statement metrics for SQL_IDs: %w", err)
		}
		statementMetricsAll = append(statementMetricsAll, statementMetrics...)
		log.Tracef("all collected metrics: %+v", statementMetricsAll)
		SQLCount = len(statementMetricsAll)
		sender.Count("dd.oracle.statements_metrics.sql_count", float64(SQLCount), "", c.tags)

		newCache := make(map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB)
		if c.statementMetricsMonotonicCountsPrevious == nil {
			c.copyToPreviousMap(newCache)
			return 0, nil
		}

		o := obfuscate.NewObfuscator(obfuscate.Config{SQL: c.config.ObfuscatorOptions})
		var diff OracleRowMonotonicCount

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

			var queryHashCol string
			//if statementMetricRow.StatementMetricsKeyDB.ForceMatchingSignature == 0 {
			if statementMetricRow.StatementMetricsKeyDB.ForceMatchingSignature == "" {
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
				log.Errorf("query metrics statements error named exec %s ", err)
				SQLTextErrors++
				continue
			}

			var SQLStatement string
			rows.Next()
			cols, err := rows.SliceScan()
			rows.Close()
			if err != nil {
				log.Errorf("query metrics statement scan error %s %s %+v", err, SQLTextQuery, p)
				SQLTextErrors++
				continue
			}
			SQLStatement = cols[0].(string)
			sender.Histogram("dd.oracle.statements_metrics.sql_text_length", float64(len(SQLStatement)), "", c.tags)

			queryRow := QueryRow{}
			//obfuscatedStatement, err := c.GetObfuscatedStatement(o, SQLStatement, statementMetricRow.ForceMatchingSignature, statementMetricRow.SQLID)
			obfuscatedStatement, err := c.GetObfuscatedStatement(o, SQLStatement)
			SQLStatement = obfuscatedStatement.Statement
			if err == nil {
				queryRow.QuerySignature = obfuscatedStatement.QuerySignature
				queryRow.Commands = obfuscatedStatement.Commands
				queryRow.Tables = obfuscatedStatement.Tables
			}

			var queryHash string
			if queryHashCol == "force_matching_signature" {
				//queryHash = strconv.FormatUint(statementMetricRow.StatementMetricsKeyDB.ForceMatchingSignature, 10)
				queryHash = statementMetricRow.StatementMetricsKeyDB.ForceMatchingSignature
			} else {
				queryHash = statementMetricRow.StatementMetricsKeyDB.SQLID
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
		}
		o.Stop()
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
	payload := MetricsPayload{
		Host:                  c.dbHostname,
		Timestamp:             float64(time.Now().UnixMilli()),
		MinCollectionInterval: c.checkInterval,
		Tags:                  c.tags,
		AgentVersion:          c.agentVersion,
		//AgentHostname:         c.hostname,
		OracleRows:    oracleRows,
		OracleVersion: c.dbVersion,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("Error marshalling query metrics payload: %s", err)
		return 0, err
	}

	log.Tracef("Query metrics payload %s", strings.ReplaceAll(string(payloadBytes), "@", "XX"))

	sender.EventPlatformEvent(string(payloadBytes), "dbm-metrics")
	sender.Gauge("dd.oracle.statements_metrics.sql_text_errors", float64(SQLTextErrors), "", c.tags)
	sender.Gauge("dd.oracle.statements_metrics.time_ms", float64(time.Since(start).Milliseconds()), "", c.tags)
	sender.Commit()

	return SQLCount, nil
}
