// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

/*
 * We are selecting from sql_fulltext instead of sql_text because sql_text doesn't preserve the new lines.
 * sql_fulltext, despite "full" in its name, truncates the text after the first 1000 characters.
 * For such statements, we will have to get the text from v$sql which has the complete text.
 */
const queryFmsRandom21c = `SELECT /* DD_QM_FMS */ s.con_id con_id, c.name pdb_name, s.force_matching_signature, plan_hash_value, max(dbms_lob.substr(sql_fulltext, 1000, 1)) sql_text, max(length(sql_text)) sql_text_length, max(s.sql_id) sql_id, 
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
WHERE s.con_id = c.con_id (+) AND force_matching_signature != 0
GROUP BY s.con_id, c.name, force_matching_signature, plan_hash_value 
HAVING MAX (last_active_time) > sysdate - :seconds/24/60/60
FETCH FIRST :limit ROWS ONLY`

// 18c doesn't have `avoided_executions` column
const queryFmsRandom18c = `SELECT /* DD_QM_FMS */ s.con_id con_id, c.name pdb_name, s.force_matching_signature, plan_hash_value, max(dbms_lob.substr(sql_fulltext, 1000, 1)) sql_text, max(length(sql_text)) sql_text_length, max(s.sql_id) sql_id, 
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
 sum(io_cell_offload_returned_bytes) as io_cell_offload_returned_bytes
FROM v$sqlstats s, v$containers c 
WHERE s.con_id = c.con_id (+) AND force_matching_signature != 0
GROUP BY s.con_id, c.name, force_matching_signature, plan_hash_value 
HAVING MAX (last_active_time) > sysdate - :seconds/24/60/60
FETCH FIRST :limit ROWS ONLY`

// 12.1 doesn't have `direct_reads` column
const queryFmsRandom121 = `SELECT /* DD_QM_FMS */ s.con_id con_id, c.name pdb_name, s.force_matching_signature, plan_hash_value, max(dbms_lob.substr(sql_fulltext, 1000, 1)) sql_text, max(length(sql_text)) sql_text_length, max(s.sql_id) sql_id, 
 sum(parse_calls) as parse_calls,
 sum(disk_reads) as disk_reads,
 sum(direct_writes) as direct_writes,
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
 sum(io_cell_offload_returned_bytes) as io_cell_offload_returned_bytes
FROM v$sqlstats s, v$containers c 
WHERE s.con_id = c.con_id (+) AND force_matching_signature != 0
GROUP BY s.con_id, c.name, force_matching_signature, plan_hash_value 
HAVING MAX (last_active_time) > sysdate - :seconds/24/60/60
FETCH FIRST :limit ROWS ONLY`

// `FETCH FIRST“ doesn't exist in Oracle 11. Also, no container awarness.
const queryFmsRandom11 = `SELECT /* DD_QM_FMS */ 
	force_matching_signature, plan_hash_value, 
	sql_text, sql_text_length, sql_id, 
	parse_calls,
	disk_reads,
	direct_writes,
	buffer_gets,
	rows_processed,
	serializable_aborts,
	fetches,
	executions,
	end_of_fetch_count,
	loads,
	version_count,
	invalidations,
	px_servers_executions,
	cpu_time,
	elapsed_time,
	application_wait_time,
	concurrency_wait_time,
	cluster_wait_time,
	user_io_wait_time,
	plsql_exec_time,
	java_exec_time,
	sorts,
	sharable_mem,
	typecheck_mem,
	io_cell_offload_eligible_bytes,
	io_interconnect_bytes,
	physical_read_requests,
	physical_read_bytes,
	physical_write_requests,
	physical_write_bytes,
	io_cell_uncompressed_bytes,
	io_cell_offload_returned_bytes
FROM (SELECT
 s.force_matching_signature, plan_hash_value, 
 max(dbms_lob.substr(sql_fulltext, 1000, 1)) sql_text, max(length(sql_text)) sql_text_length, max(s.sql_id) sql_id, 
 sum(parse_calls) as parse_calls,
 sum(disk_reads) as disk_reads,
 sum(direct_writes) as direct_writes,
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
 sum(io_cell_offload_returned_bytes) as io_cell_offload_returned_bytes
FROM v$sqlstats s
WHERE force_matching_signature != 0
GROUP BY force_matching_signature, plan_hash_value 
HAVING MAX (last_active_time) > sysdate - :seconds/24/60/60 )
WHERE ROWNUM <= :limit`

// queryForceMatchingSignatureLastActive Querying force_matching_signature = 0
const queryForceMatchingSignatureLastActive21c = `SELECT /* DD_QM_FMS */ s.con_id con_id, c.name pdb_name, s.force_matching_signature, plan_hash_value, 
 max(dbms_lob.substr(sql_fulltext, 1000, 1)) sql_text, max(length(sql_text)) sql_text_length, sq.sql_id,
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
FROM v$sqlstats s, v$containers c, ( 
 SELECT * 
 FROM ( 
	 SELECT force_matching_signature, sql_id, row_number ( ) over ( partition by force_matching_signature ORDER BY last_active_time DESC ) rowno
 FROM v$sqlstats 
 WHERE last_active_time > sysdate - :seconds/24/60/60 AND force_matching_signature != 0
) 
WHERE rowno = 1
) sq 
WHERE s.con_id = c.con_id (+) AND sq.force_matching_signature = s.force_matching_signature 
GROUP BY s.con_id, c.name, s.force_matching_signature, plan_hash_value, sq.sql_id 
FETCH FIRST :limit ROWS ONLY`

const queryForceMatchingSignatureLastActive18c = `SELECT /* DD_QM_FMS */ s.con_id con_id, c.name pdb_name, s.force_matching_signature, plan_hash_value, 
 max(dbms_lob.substr(sql_fulltext, 1000, 1)) sql_text, max(length(sql_text)) sql_text_length, sq.sql_id,
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
 sum(io_cell_offload_returned_bytes) as io_cell_offload_returned_bytes
FROM v$sqlstats s, v$containers c, ( 
 SELECT * 
 FROM ( 
	 SELECT force_matching_signature, sql_id, row_number ( ) over ( partition by force_matching_signature ORDER BY last_active_time DESC ) rowno
 FROM v$sqlstats 
 WHERE last_active_time > sysdate - :seconds/24/60/60 AND force_matching_signature != 0
) 
WHERE rowno = 1
) sq 
WHERE s.con_id = c.con_id (+) AND sq.force_matching_signature = s.force_matching_signature 
GROUP BY s.con_id, c.name, s.force_matching_signature, plan_hash_value, sq.sql_id 
FETCH FIRST :limit ROWS ONLY`

const queryForceMatchingSignatureLastActive121 = `SELECT /* DD_QM_FMS */ s.con_id con_id, c.name pdb_name, s.force_matching_signature, plan_hash_value, 
 max(dbms_lob.substr(sql_fulltext, 1000, 1)) sql_text, max(length(sql_text)) sql_text_length, sq.sql_id,
 sum(parse_calls) as parse_calls,
 sum(disk_reads) as disk_reads,
 sum(direct_writes) as direct_writes,
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
 sum(io_cell_offload_returned_bytes) as io_cell_offload_returned_bytes
FROM v$sqlstats s, v$containers c, ( 
 SELECT * 
 FROM ( 
	 SELECT force_matching_signature, sql_id, row_number ( ) over ( partition by force_matching_signature ORDER BY last_active_time DESC ) rowno
 FROM v$sqlstats 
 WHERE last_active_time > sysdate - :seconds/24/60/60 AND force_matching_signature != 0
) 
WHERE rowno = 1
) sq 
WHERE s.con_id = c.con_id (+) AND sq.force_matching_signature = s.force_matching_signature 
GROUP BY s.con_id, c.name, s.force_matching_signature, plan_hash_value, sq.sql_id 
FETCH FIRST :limit ROWS ONLY`

const queryForceMatchingSignatureLastActive11 = `SELECT /* DD_QM_FMS */ 
	force_matching_signature, plan_hash_value, 
	sql_text, sql_text_length, sql_id,
	parse_calls,
	disk_reads,
	direct_writes,
	buffer_gets,
	rows_processed,
	serializable_aborts,
	fetches,
	executions,
	end_of_fetch_count,
	loads,
	version_count,
	invalidations,
	px_servers_executions,
	cpu_time,
	elapsed_time,
	application_wait_time,
	concurrency_wait_time,
	cluster_wait_time,
	user_io_wait_time,
	plsql_exec_time,
	java_exec_time,
	sorts,
	sharable_mem,
	typecheck_mem,
	io_cell_offload_eligible_bytes,
	io_interconnect_bytes,
	physical_read_requests,
	physical_read_bytes,
	physical_write_requests,
	physical_write_bytes,
	io_cell_uncompressed_bytes,
	io_cell_offload_returned_bytes
FROM (SELECT
 s.force_matching_signature, plan_hash_value, 
 max(dbms_lob.substr(sql_fulltext, 1000, 1)) sql_text, max(length(sql_text)) sql_text_length, sq.sql_id,
 sum(parse_calls) as parse_calls,
 sum(disk_reads) as disk_reads,
 sum(direct_writes) as direct_writes,
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
 sum(io_cell_offload_returned_bytes) as io_cell_offload_returned_bytes
FROM v$sqlstats s, ( 
 SELECT * 
 FROM ( 
	 SELECT force_matching_signature, sql_id, row_number ( ) over ( partition by force_matching_signature ORDER BY last_active_time DESC ) rowno
 FROM v$sqlstats 
 WHERE last_active_time > sysdate - :seconds/24/60/60 AND force_matching_signature != 0
) 
WHERE rowno = 1
) sq 
WHERE sq.force_matching_signature = s.force_matching_signature 
GROUP BY s.force_matching_signature, plan_hash_value, sq.sql_id )
WHERE ROWNUM <= :limit`

// querySQLID force_matching_signature = 0
const querySQLID21c = `SELECT /* DD_QM_SQLID */ s.con_id con_id, c.name pdb_name, sql_id, plan_hash_value, 
 dbms_lob.substr(sql_fulltext, 1000, 1) sql_text, length(sql_text) sql_text_length, 
 parse_calls,
 disk_reads,
 direct_writes,
 direct_reads,
 buffer_gets,
 rows_processed,
 serializable_aborts,
 fetches,
 executions,
 end_of_fetch_count,
 loads,
 version_count,
 invalidations,
 px_servers_executions,
 cpu_time,
 elapsed_time,
 application_wait_time,
 concurrency_wait_time,
 cluster_wait_time,
 user_io_wait_time,
 plsql_exec_time,
 java_exec_time,
 sorts,
 sharable_mem,
 typecheck_mem,
 io_cell_offload_eligible_bytes,
 io_interconnect_bytes,
 physical_read_requests,
 physical_read_bytes,
 physical_write_requests,
 physical_write_bytes,
 io_cell_uncompressed_bytes,
 io_cell_offload_returned_bytes,
 avoided_executions
FROM v$sqlstats s, v$containers c 
WHERE s.con_id = c.con_id (+) AND last_active_time > sysdate - :seconds/24/60/60 AND force_matching_signature = 0
FETCH FIRST :limit ROWS ONLY`

const querySQLID18c = `SELECT /* DD_QM_SQLID */ s.con_id con_id, c.name pdb_name, sql_id, plan_hash_value, 
 dbms_lob.substr(sql_fulltext, 1000, 1) sql_text, length(sql_text) sql_text_length, 
 parse_calls,
 disk_reads,
 direct_writes,
 direct_reads,
 buffer_gets,
 rows_processed,
 serializable_aborts,
 fetches,
 executions,
 end_of_fetch_count,
 loads,
 version_count,
 invalidations,
 px_servers_executions,
 cpu_time,
 elapsed_time,
 application_wait_time,
 concurrency_wait_time,
 cluster_wait_time,
 user_io_wait_time,
 plsql_exec_time,
 java_exec_time,
 sorts,
 sharable_mem,
 typecheck_mem,
 io_cell_offload_eligible_bytes,
 io_interconnect_bytes,
 physical_read_requests,
 physical_read_bytes,
 physical_write_requests,
 physical_write_bytes,
 io_cell_uncompressed_bytes,
 io_cell_offload_returned_bytes
FROM v$sqlstats s, v$containers c 
WHERE s.con_id = c.con_id (+) AND last_active_time > sysdate - :seconds/24/60/60 AND force_matching_signature = 0
FETCH FIRST :limit ROWS ONLY`

const querySQLID121 = `SELECT /* DD_QM_SQLID */ s.con_id con_id, c.name pdb_name, sql_id, plan_hash_value, 
 dbms_lob.substr(sql_fulltext, 1000, 1) sql_text, length(sql_text) sql_text_length, 
 parse_calls,
 disk_reads,
 direct_writes,
 buffer_gets,
 rows_processed,
 serializable_aborts,
 fetches,
 executions,
 end_of_fetch_count,
 loads,
 version_count,
 invalidations,
 px_servers_executions,
 cpu_time,
 elapsed_time,
 application_wait_time,
 concurrency_wait_time,
 cluster_wait_time,
 user_io_wait_time,
 plsql_exec_time,
 java_exec_time,
 sorts,
 sharable_mem,
 typecheck_mem,
 io_cell_offload_eligible_bytes,
 io_interconnect_bytes,
 physical_read_requests,
 physical_read_bytes,
 physical_write_requests,
 physical_write_bytes,
 io_cell_uncompressed_bytes,
 io_cell_offload_returned_bytes
FROM v$sqlstats s, v$containers c 
WHERE s.con_id = c.con_id (+) AND last_active_time > sysdate - :seconds/24/60/60 AND force_matching_signature = 0
FETCH FIRST :limit ROWS ONLY`

const querySQLID11 = `SELECT /* DD_QM_SQLID */ sql_id, plan_hash_value, 
 dbms_lob.substr(sql_fulltext, 1000, 1) sql_text, length(sql_text) sql_text_length, 
 parse_calls,
 disk_reads,
 direct_writes,
 buffer_gets,
 rows_processed,
 serializable_aborts,
 fetches,
 executions,
 end_of_fetch_count,
 loads,
 version_count,
 invalidations,
 px_servers_executions,
 cpu_time,
 elapsed_time,
 application_wait_time,
 concurrency_wait_time,
 cluster_wait_time,
 user_io_wait_time,
 plsql_exec_time,
 java_exec_time,
 sorts,
 sharable_mem,
 typecheck_mem,
 io_cell_offload_eligible_bytes,
 io_interconnect_bytes,
 physical_read_requests,
 physical_read_bytes,
 physical_write_requests,
 physical_write_bytes,
 io_cell_uncompressed_bytes,
 io_cell_offload_returned_bytes
FROM v$sqlstats s
WHERE last_active_time > sysdate - :seconds/24/60/60 AND force_matching_signature = 0
 AND ROWNUM <= :limit`

type statementMetricsQuery int

const (
	fmsRandomQuery statementMetricsQuery = iota
	fmsLastActiveQuery
	sqlIDQuery
)

func getStatementMetricsQueries(c *Check) map[statementMetricsQuery]string {
	queries := make(map[statementMetricsQuery]string)
	if isDbVersionLessThan(c, "12") {
		queries[fmsRandomQuery] = queryFmsRandom11
		queries[fmsLastActiveQuery] = queryForceMatchingSignatureLastActive11
		queries[sqlIDQuery] = querySQLID11
	} else if isDbVersionLessThan(c, "12.2") {
		queries[fmsRandomQuery] = queryFmsRandom121
		queries[fmsLastActiveQuery] = queryForceMatchingSignatureLastActive121
		queries[sqlIDQuery] = querySQLID121
	} else if isDbVersionLessThan(c, "19") {
		queries[fmsRandomQuery] = queryFmsRandom18c
		queries[fmsLastActiveQuery] = queryForceMatchingSignatureLastActive18c
		queries[sqlIDQuery] = querySQLID18c
	} else {
		queries[fmsRandomQuery] = queryFmsRandom21c
		queries[fmsLastActiveQuery] = queryForceMatchingSignatureLastActive21c
		queries[sqlIDQuery] = querySQLID21c
	}
	return queries
}
