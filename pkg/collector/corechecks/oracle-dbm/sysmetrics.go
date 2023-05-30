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

const SYSMETRICS_QUERY = `SELECT metric_name, value, name pdb_name 
  FROM %s s, v$containers c 
  WHERE s.con_id = c.con_id(+)
  ORDER BY begin_time DESC`

type SysmetricsRowDB struct {
	MetricName string         `db:"METRIC_NAME"`
	Value      float64        `db:"VALUE"`
	PdbName    sql.NullString `db:"PDB_NAME"`
}

type sysMetricsDefinition struct {
	DDmetric               string
	DBM                    bool
	seenInContainerMetrics bool
}

func (c *Check) SysMetrics() error {
	SYSMETRICS_COLS := map[string]sysMetricsDefinition{
		"Buffer Cache Hit Ratio":          {DDmetric: "buffer_cachehit_ratio"},
		"Cursor Cache Hit Ratio":          {DDmetric: "cursor_cachehit_ratio"},
		"Library Cache Hit Ratio":         {DDmetric: "library_cachehit_ratio"},
		"Shared Pool Free %":              {DDmetric: "shared_pool_free"},
		"Physical Reads Per Sec":          {DDmetric: "physical_reads"},
		"Physical Writes Per Sec":         {DDmetric: "physical_writes"},
		"Enqueue Timeouts Per Sec":        {DDmetric: "enqueue_timeouts"},
		"GC CR Block Received Per Second": {DDmetric: "gc_cr_block_received"},
		"Global Cache Blocks Corrupted":   {DDmetric: "cache_blocks_corrupt"},
		"Global Cache Blocks Lost":        {DDmetric: "cache_blocks_lost"},
		"Logons Per Sec":                  {DDmetric: "logons"},
		"Average Active Sessions":         {DDmetric: "active_sessions"},
		"Long Table Scans Per Sec":        {DDmetric: "long_table_scans"},
		"SQL Service Response Time":       {DDmetric: "service_response_time"},
		"User Rollbacks Per Sec":          {DDmetric: "user_rollbacks"},
		"Total Sorts Per User Call":       {DDmetric: "sorts_per_user_call"},
		"Rows Per Sort":                   {DDmetric: "rows_per_sort"},
		"Disk Sort Per Sec":               {DDmetric: "disk_sorts"},
		"Memory Sorts Ratio":              {DDmetric: "memory_sorts_ratio"},
		"Database Wait Time Ratio":        {DDmetric: "database_wait_time_ratio"},
		"Session Limit %":                 {DDmetric: "session_limit_usage"},
		"Session Count":                   {DDmetric: "session_count"},
		"Temp Space Used":                 {DDmetric: "temp_space_used"},
	}

	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}

	metricRows := []SysmetricsRowDB{}
	err = c.db.Select(&metricRows, fmt.Sprintf(SYSMETRICS_QUERY, "v$con_sysmetric"))
	if err != nil {
		return fmt.Errorf("failed to collect container sysmetrics: %w", err)
	}
	seenInContainerMetrics := make(map[string]bool)
	for _, r := range metricRows {
		if metric, ok := SYSMETRICS_COLS[r.MetricName]; ok {
			sender.Gauge(fmt.Sprintf("%s.%s", common.IntegrationName, metric.DDmetric), r.Value, "", appendPDBTag(c.tags, r.PdbName))
			seenInContainerMetrics[r.MetricName] = true
		}
	}

	err = c.db.Select(&metricRows, fmt.Sprintf(SYSMETRICS_QUERY, "v$sysmetric"))
	if err != nil {
		return fmt.Errorf("failed to collect sysmetrics: %w", err)
	}
	for _, r := range metricRows {
		if metric, ok := SYSMETRICS_COLS[r.MetricName]; ok {
			if _, ok := seenInContainerMetrics[r.MetricName]; !ok {
				sender.Gauge(fmt.Sprintf("%s.%s", common.IntegrationName, metric.DDmetric), r.Value, "", appendPDBTag(c.tags, r.PdbName))
			}
		}
	}

	sender.Commit()
	return nil
}
