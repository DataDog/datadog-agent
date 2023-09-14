// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const SYSMETRICS_QUERY = `SELECT 
	metric_name,
	value, 
	metric_unit, 
	name pdb_name 
  FROM %s s, v$containers c 
  WHERE s.con_id = c.con_id(+)`

const (
	Count int = 0
)

type SysmetricsRowDB struct {
	MetricName string         `db:"METRIC_NAME"`
	Value      float64        `db:"VALUE"`
	MetricUnit string         `db:"METRIC_UNIT"`
	PdbName    sql.NullString `db:"PDB_NAME"`
}

type sysMetricsDefinition struct {
	DDmetric string
	DBM      bool
}

var SYSMETRICS_COLS = map[string]sysMetricsDefinition{
	"Average Active Sessions":                       {DDmetric: "active_sessions"},
	"Average Synchronous Single-Block Read Latency": {DDmetric: "avg_synchronous_single_block_read_latency", DBM: true},
	"Background CPU Usage Per Sec":                  {DDmetric: "active_background_on_cpu", DBM: true},
	"Background Time Per Sec":                       {DDmetric: "active_background", DBM: true},
	"Branch Node Splits Per Sec":                    {DDmetric: "branch_node_splits", DBM: true},
	"Buffer Cache Hit Ratio":                        {DDmetric: "buffer_cachehit_ratio"},
	"Consistent Read Changes Per Sec":               {DDmetric: "consistent_read_changes", DBM: true},
	"Consistent Read Gets Per Sec":                  {DDmetric: "consistent_read_gets", DBM: true},
	"CPU Usage Per Sec":                             {DDmetric: "active_sessions_on_cpu", DBM: true},
	"Current OS Load":                               {DDmetric: "os_load", DBM: true},
	//"Cursor Cache Hit Ratio":                        {DDmetric: "cursor_cachehit_ratio"},
	"Database CPU Time Ratio":                  {DDmetric: "database_cpu_time_ratio", DBM: true},
	"Database Wait Time Ratio":                 {DDmetric: "database_wait_time_ratio"},
	"DB Block Changes Per Sec":                 {DDmetric: "db_block_changes", DBM: true},
	"DB Block Gets Per Sec":                    {DDmetric: "db_block_gets", DBM: true},
	"DBWR Checkpoints Per Sec":                 {DDmetric: "dbwr_checkpoints", DBM: true},
	"Disk Sort Per Sec":                        {DDmetric: "disk_sorts"},
	"Enqueue Deadlocks Per Sec":                {DDmetric: "enqueue_deadlocks", DBM: true},
	"Enqueue Timeouts Per Sec":                 {DDmetric: "enqueue_timeouts"},
	"Execute Without Parse Ratio":              {DDmetric: "execute_without_parse", DBM: true},
	"GC CR Block Received Per Second":          {DDmetric: "gc_cr_block_received"},
	"GC Current Block Received Per Second":     {DDmetric: "gc_current_block_received", DBM: true},
	"Global Cache Average CR Get Time":         {DDmetric: "gc_average_cr_get_time", DBM: true},
	"Global Cache Average Current Get Time":    {DDmetric: "gc_average_current_get_time", DBM: true},
	"Global Cache Blocks Corrupted":            {DDmetric: "cache_blocks_corrupt"},
	"Global Cache Blocks Lost":                 {DDmetric: "cache_blocks_lost"},
	"Hard Parse Count Per Sec":                 {DDmetric: "hard_parses", DBM: true},
	"Host CPU Utilization (%)":                 {DDmetric: "host_cpu_utilization", DBM: true},
	"Leaf Node Splits Per Sec":                 {DDmetric: "leaf_nodes_splits", DBM: true},
	"Library Cache Hit Ratio":                  {DDmetric: "library_cachehit_ratio"},
	"Logical Reads Per Sec":                    {DDmetric: "logical_reads", DBM: true},
	"Logons Per Sec":                           {DDmetric: "logons"},
	"Long Table Scans Per Sec":                 {DDmetric: "long_table_scans"},
	"Memory Sorts Ratio":                       {DDmetric: "memory_sorts_ratio"},
	"Network Traffic Volume Per Sec":           {DDmetric: "network_traffic_volume", DBM: true},
	"PGA Cache Hit %":                          {DDmetric: "pga_cache_hit", DBM: true},
	"Parse Failure Count Per Sec":              {DDmetric: "parse_failures", DBM: true},
	"Physical Read Bytes Per Sec":              {DDmetric: "physical_read_bytes", DBM: true},
	"Physical Read IO Requests Per Sec":        {DDmetric: "physical_read_io_requests", DBM: true},
	"Physical Read Total IO Requests Per Sec":  {DDmetric: "physical_read_total_io_requests", DBM: true},
	"Physical Reads Direct Lobs Per Sec":       {DDmetric: "physical_reads_direct_lobs", DBM: true},
	"Physical Read Total Bytes Per Sec":        {DDmetric: "physical_read_total_bytes", DBM: true},
	"Physical Reads Direct Per Sec":            {DDmetric: "physical_reads_direct", DBM: true},
	"Physical Reads Per Sec":                   {DDmetric: "physical_reads"},
	"Physical Write Bytes Per Sec":             {DDmetric: "physical_write_bytes", DBM: true},
	"Physical Write IO Requests Per Sec":       {DDmetric: "physical_write_io_requests", DBM: true},
	"Physical Write Total Bytes Per Sec":       {DDmetric: "physical_write_total_bytes", DBM: true},
	"Physical Write Total IO Requests Per Sec": {DDmetric: "physical_write_total_io_requests", DBM: true},
	"Physical Writes Direct Lobs Per Sec":      {DDmetric: "physical_writes_direct_lobs", DBM: true},
	"Physical Writes Direct Per Sec":           {DDmetric: "physical_writes_direct", DBM: true},
	"Physical Writes Per Sec":                  {DDmetric: "physical_writes"},
	"Process Limit %":                          {DDmetric: "process_limit", DBM: true},
	"Redo Allocation Hit Ratio":                {DDmetric: "redo_allocation_hit_ratio", DBM: true},
	"Redo Generated Per Sec":                   {DDmetric: "redo_generated", DBM: true},
	"Redo Writes Per Sec":                      {DDmetric: "redo_writes", DBM: true},
	"Row Cache Hit Ratio":                      {DDmetric: "row_cache_hit_ratio", DBM: true},
	"Rows Per Sort":                            {DDmetric: "rows_per_sort"},
	"SQL Service Response Time":                {DDmetric: "service_response_time"},
	"Session Count":                            {DDmetric: "session_count"},
	"Session Limit %":                          {DDmetric: "session_limit_usage"},
	"Shared Pool Free %":                       {DDmetric: "shared_pool_free"},
	"Soft Parse Ratio":                         {DDmetric: "soft_parse_ratio", DBM: true},
	"Temp Space Used":                          {DDmetric: "temp_space_used"},
	"Total Parse Count Per Sec":                {DDmetric: "total_parse_count", DBM: true},
	"Total Sorts Per User Call":                {DDmetric: "sorts_per_user_call"},
	"User Commits Per Sec":                     {DDmetric: "user_commits", DBM: true},
	"User Rollbacks Per Sec":                   {DDmetric: "user_rollbacks"},
}

func (c *Check) sendMetric(s sender.Sender, r SysmetricsRowDB, seen map[string]bool) {
	if metric, ok := SYSMETRICS_COLS[r.MetricName]; ok {
		value := r.Value
		if r.MetricUnit == "CentiSeconds Per Second" {
			value = value / 100
		}
		if !SYSMETRICS_COLS[r.MetricName].DBM || SYSMETRICS_COLS[r.MetricName].DBM && c.dbmEnabled {
			log.Tracef("%s: %f", metric.DDmetric, value)
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
	err = selectWrapper(c, &metricRows, fmt.Sprintf(SYSMETRICS_QUERY, "v$con_sysmetric"))
	if err != nil {
		return fmt.Errorf("failed to collect container sysmetrics: %w", err)
	}
	seenInContainerMetrics := make(map[string]bool)
	for _, r := range metricRows {
		c.sendMetric(sender, r, seenInContainerMetrics)
	}

	seenInGlobalMetrics := make(map[string]bool)
	err = selectWrapper(c, &metricRows, fmt.Sprintf(SYSMETRICS_QUERY, "v$sysmetric")+" ORDER BY begin_time ASC, metric_name ASC")
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

	var overAllocationCount float64
	err = getWrapper(c, &overAllocationCount, "SELECT value FROM v$pgastat WHERE name = 'over allocation count'")
	if err != nil {
		return fmt.Errorf("failed to get PGA over allocation count: %w", err)
	}
	if c.previousPGAOverAllocationCount.valid {
		v := overAllocationCount - c.previousPGAOverAllocationCount.value
		sender.Gauge(fmt.Sprintf("%s.%s", common.IntegrationName, "pga_over_allocation_count"), v, "", c.tags)
		c.previousPGAOverAllocationCount.value = overAllocationCount
	} else {
		c.previousPGAOverAllocationCount = pgaOverAllocationCount{value: overAllocationCount, valid: true}
	}

	sender.Commit()
	return nil
}
