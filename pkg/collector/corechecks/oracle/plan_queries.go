// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

// including sql_id for indexed access
const planQuery12 = `SELECT /* DD */
  child_number,
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
	last_tempseg_size
FROM v$sql_plan_statistics_all s
WHERE 
  sql_id = :1 AND plan_hash_value = :2 AND con_id = :3
ORDER BY timestamp desc, child_number, id, position`

const planQuery11 = `SELECT /* DD */
  child_number,
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
	last_tempseg_size
FROM v$sql_plan_statistics_all s
WHERE 
  sql_id = :1 AND plan_hash_value = :2
ORDER BY timestamp desc, child_number, id, position`
