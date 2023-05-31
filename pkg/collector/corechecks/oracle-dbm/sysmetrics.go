// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"database/sql"
	"fmt"
	"math"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
)

const SYSMETRICS_QUERY = `SELECT 
	metric_name,
	value, 
	metric_unit, 
	(end_time - begin_time)*24*3600 interval_length,
	name pdb_name 
  FROM %s s, v$containers c 
  WHERE s.con_id = c.con_id(+)`

const (
	Count int = 0
)

type SysmetricsRowDB struct {
	MetricName     string         `db:"METRIC_NAME"`
	Value          float64        `db:"VALUE"`
	MetricUnit     string         `db:"METRIC_UNIT"`
	IntervalLength float64        `db:"INTERVAL_LENGTH"`
	PdbName        sql.NullString `db:"PDB_NAME"`
}

type sysMetricsDefinition struct {
	DDmetric    string
	DBM         bool
	PostProcess int
}

var SYSMETRICS_COLS = map[string]sysMetricsDefinition{
	"Average Active Sessions":                       {DDmetric: "active_sessions"},
	"Average Synchronous Single-Block Read Latency": {DDmetric: "avg_synchronous_single_block_read_latency", DBM: true},
	"Background CPU Usage Per Sec":                  {DDmetric: "active_background_on_cpu", DBM: true},
	"Background Time Per Sec":                       {DDmetric: "active_background", DBM: true},
	"Branch Node Splits Per Sec":                    {DDmetric: "branch_node_splits", DBM: true, PostProcess: Count},
	"Buffer Cache Hit Ratio":                        {DDmetric: "buffer_cachehit_ratio"},
	"CPU Usage Per Sec":                             {DDmetric: "active_sessions_on_cpu", DBM: true},
	"Cursor Cache Hit Ratio":                        {DDmetric: "cursor_cachehit_ratio"},
	"Library Cache Hit Ratio":                       {DDmetric: "library_cachehit_ratio"},
	"Shared Pool Free %":                            {DDmetric: "shared_pool_free"},
	"Physical Reads Per Sec":                        {DDmetric: "physical_reads"},
	"Physical Writes Per Sec":                       {DDmetric: "physical_writes"},
	"Enqueue Timeouts Per Sec":                      {DDmetric: "enqueue_timeouts"},
	"GC CR Block Received Per Second":               {DDmetric: "gc_cr_block_received"},
	"Global Cache Blocks Corrupted":                 {DDmetric: "cache_blocks_corrupt"},
	"Global Cache Blocks Lost":                      {DDmetric: "cache_blocks_lost"},
	"Logons Per Sec":                                {DDmetric: "logons"},
	"Long Table Scans Per Sec":                      {DDmetric: "long_table_scans"},
	"SQL Service Response Time":                     {DDmetric: "service_response_time"},
	"User Rollbacks Per Sec":                        {DDmetric: "user_rollbacks"},
	"Total Sorts Per User Call":                     {DDmetric: "sorts_per_user_call"},
	"Rows Per Sort":                                 {DDmetric: "rows_per_sort"},
	"Disk Sort Per Sec":                             {DDmetric: "disk_sorts"},
	"Memory Sorts Ratio":                            {DDmetric: "memory_sorts_ratio"},
	"Database Wait Time Ratio":                      {DDmetric: "database_wait_time_ratio"},
	"Session Limit %":                               {DDmetric: "session_limit_usage"},
	"Session Count":                                 {DDmetric: "session_count"},
	"Temp Space Used":                               {DDmetric: "temp_space_used"},
}

func (c *Check) sendMetric(s aggregator.Sender, r SysmetricsRowDB, seen map[string]bool) {
	if metric, ok := SYSMETRICS_COLS[r.MetricName]; ok {
		value := r.Value
		if r.MetricUnit == "CentiSeconds Per Second" {
			value = value / 100
		}
		if SYSMETRICS_COLS[r.MetricName].PostProcess == Count {
			value = math.Round(value * r.IntervalLength)
		}
		if !SYSMETRICS_COLS[r.MetricName].DBM || SYSMETRICS_COLS[r.MetricName].DBM && c.dbmEnabled {
			s.Gauge(fmt.Sprintf("%s.%s", common.IntegrationName, metric.DDmetric), value, "", appendPDBTag(c.tags, r.PdbName))
			seen[r.MetricName] = true
		}
	}
}

func (c *Check) SysMetrics() error {

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
		c.sendMetric(sender, r, seenInContainerMetrics)
	}

	seenInGlobalMetrics := make(map[string]bool)
	err = c.db.Select(&metricRows, fmt.Sprintf(SYSMETRICS_QUERY, "v$sysmetric")+" ORDER BY begin_time ASC, metric_name ASC")
	if err != nil {
		return fmt.Errorf("failed to collect sysmetrics: %w", err)
	}
	for _, r := range metricRows {
		if _, ok := seenInContainerMetrics[r.MetricName]; !ok {
			if _, ok := seenInGlobalMetrics[r.MetricName]; ok {
				break
			} else {
				c.sendMetric(sender, r, seenInGlobalMetrics)
			}
		}
	}

	sender.Commit()
	return nil
}
