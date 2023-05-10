// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"database/sql"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
)

const SYSMETRICS_QUERY = "SELECT metric_name, value, name pdb_name FROM v$con_sysmetric s, v$containers c WHERE s.con_id = c.con_id(+)"

type SysmetricsRowDB struct {
	MetricName string         `db:"METRIC_NAME"`
	Value      float64        `db:"VALUE"`
	PdbName    sql.NullString `db:"PDB_NAME"`
}

func (c *Check) SysMetrics() error {
	SYSMETRICS_COLS := map[string]string{
		"Buffer Cache Hit Ratio":          "buffer_cachehit_ratio",
		"Cursor Cache Hit Ratio":          "cursor_cachehit_ratio",
		"Library Cache Hit Ratio":         "library_cachehit_ratio",
		"Shared Pool Free %":              "shared_pool_free",
		"Physical Reads Per Sec":          "physical_reads",
		"Physical Writes Per Sec":         "physical_writes",
		"Enqueue Timeouts Per Sec":        "enqueue_timeouts",
		"GC CR Block Received Per Second": "gc_cr_block_received",
		"Global Cache Blocks Corrupted":   "cache_blocks_corrupt",
		"Global Cache Blocks Lost":        "cache_blocks_lost",
		"Logons Per Sec":                  "logons",
		"Average Active Sessions":         "active_sessions",
		"Long Table Scans Per Sec":        "long_table_scans",
		"SQL Service Response Time":       "service_response_time",
		"User Rollbacks Per Sec":          "user_rollbacks",
		"Total Sorts Per User Call":       "sorts_per_user_call",
		"Rows Per Sort":                   "rows_per_sort",
		"Disk Sort Per Sec":               "disk_sorts",
		"Memory Sorts Ratio":              "memory_sorts_ratio",
		"Database Wait Time Ratio":        "database_wait_time_ratio",
		"Session Limit %":                 "session_limit_usage",
		"Session Count":                   "session_count",
		"Temp Space Used":                 "temp_space_used",
	}

	sysMetrics := []SysmetricsRowDB{}
	err := c.db.Select(&sysMetrics, SYSMETRICS_QUERY)

	if err != nil {
		return fmt.Errorf("failed to collect sysmetrics: %w", err)
	}
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("GetSender sysmetrics %w", err)
	}
	for _, metricRow := range sysMetrics {
		DDMetricName, mustSend := SYSMETRICS_COLS[metricRow.MetricName]
		if !mustSend {
			continue
		}
		sender.Gauge(fmt.Sprintf("%s.%s", common.IntegrationName, DDMetricName), metricRow.Value, "", c.getTagsWithPDB(metricRow.PdbName))
	}
	sender.Commit()
	return nil
}
