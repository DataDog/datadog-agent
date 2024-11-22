// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

const activityQueryOnView12 = `SELECT /* DD_ACTIVITY_SAMPLING */
	SYSDATE as now,
	sid,
	serial#,
	username,
	status,
    osuser,
    process,
    machine,
	port,
    program,
    type,
    sql_id,
	force_matching_signature,
    sql_plan_hash_value,
    sql_exec_start,
	sql_address,
	op_flags,
	prev_sql_id,
	prev_force_matching_signature,
    prev_sql_plan_hash_value,
    prev_sql_exec_start,
	prev_sql_address,
    module,
    action,
    client_info,
    logon_time,
    client_identifier,
    CASE WHEN blocking_session_status = 'VALID' THEN
	  blocking_instance
	ELSE
	  null
	END blocking_instance,
    CASE WHEN blocking_session_status = 'VALID' THEN
		blocking_session
	ELSE
		null
	END blocking_session,
	CASE WHEN final_blocking_session_status = 'VALID' THEN
    	final_blocking_instance
	ELSE
		null
	END final_blocking_instance,
	CASE WHEN final_blocking_session_status = 'VALID' THEN
    	final_blocking_session
	ELSE
		null
	END final_blocking_session,
    CASE WHEN state = 'WAITING' THEN
		event
	ELSE
		'CPU'
	END event,
	CASE WHEN state = 'WAITING' THEN
    	wait_class
	ELSE
		'CPU'
	END wait_class,
	wait_time_micro,
	dbms_lob.substr(sql_fulltext, :sql_substr_length_1, 1) sql_fulltext,
	dbms_lob.substr(prev_sql_fulltext, :sql_substr_length_2, 1) prev_sql_fulltext,
	pdb_name,
	command_name
FROM sys.dd_session
WHERE
	( sql_text NOT LIKE '%DD_ACTIVITY_SAMPLING%' OR sql_text is NULL )`

const activityQueryOnView11 = `SELECT /* DD_ACTIVITY_SAMPLING */
	SYSDATE as now,
	sid,
	serial#,
	username,
	status,
    osuser,
    process,
    machine,
	port,
    program,
    type,
    sql_id,
	force_matching_signature,
    sql_plan_hash_value,
    sql_exec_start,
	sql_address,
	op_flags,
	prev_sql_id,
	prev_force_matching_signature,
    prev_sql_plan_hash_value,
    prev_sql_exec_start,
	prev_sql_address,
    module,
    action,
    client_info,
    logon_time,
    client_identifier,
    CASE WHEN blocking_session_status = 'VALID' THEN
	  blocking_instance
	ELSE
	  null
	END blocking_instance,
    CASE WHEN blocking_session_status = 'VALID' THEN
		blocking_session
	ELSE
		null
	END blocking_session,
	CASE WHEN final_blocking_session_status = 'VALID' THEN
    	final_blocking_instance
	ELSE
		null
	END final_blocking_instance,
	CASE WHEN final_blocking_session_status = 'VALID' THEN
    	final_blocking_session
	ELSE
		null
	END final_blocking_session,
    CASE WHEN state = 'WAITING' THEN
		event
	ELSE
		'CPU'
	END event,
	CASE WHEN state = 'WAITING' THEN
    	wait_class
	ELSE
		'CPU'
	END wait_class,
	wait_time_micro,
	dbms_lob.substr(sql_fulltext, :sql_substr_length_1, 1) sql_fulltext,
	dbms_lob.substr(prev_sql_fulltext, :sql_substr_length_2, 1) prev_sql_fulltext,
	command_name
FROM sys.dd_session
WHERE
	( sql_text NOT LIKE '%DD_ACTIVITY_SAMPLING%' OR sql_text is NULL )`

const activityQueryDirect = `SELECT /*+ push_pred(sq) push_pred(sq_prev) */ /* DD_ACTIVITY_SAMPLING */
SYSDATE as now,
s.sid,
s.serial#,
s.username,
s.status,
s.osuser,
s.process,
s.machine,
s.port,
s.program,
s.type,
s.sql_id,
sq.force_matching_signature as force_matching_signature,
sq.plan_hash_value sql_plan_hash_value,
s.sql_exec_start,
s.sql_address,
s.prev_sql_id,
sq_prev.plan_hash_value prev_sql_plan_hash_value,
s.prev_exec_start as prev_sql_exec_start,
sq_prev.force_matching_signature as prev_force_matching_signature,
s.prev_sql_addr prev_sql_address,
s.module,
s.action,
s.client_info,
s.logon_time,
s.client_identifier,
CASE WHEN blocking_session_status = 'VALID' THEN
blocking_instance
ELSE
null
END blocking_instance,
CASE WHEN blocking_session_status = 'VALID' THEN
  blocking_session
ELSE
  null
END blocking_session,
CASE WHEN final_blocking_session_status = 'VALID' THEN
  final_blocking_instance
ELSE
  null
END final_blocking_instance,
CASE WHEN final_blocking_session_status = 'VALID' THEN
  final_blocking_session
ELSE
  null
END final_blocking_session,
CASE WHEN state = 'WAITING' THEN
  event
ELSE
  'CPU'
END event,
CASE WHEN state = 'WAITING' THEN
  wait_class
ELSE
  'CPU'
END wait_class,
s.wait_time_micro,
c.name as pdb_name,
dbms_lob.substr(sq.sql_fulltext, :sql_substr_length_1, 1) sql_fulltext,
dbms_lob.substr(sq_prev.sql_fulltext, :sql_substr_length_2, 1) prev_sql_fulltext,
comm.command_name
FROM
v$session s,
v$sql sq,
v$sql sq_prev,
v$containers c,
v$sqlcommand comm
WHERE
sq.sql_id(+)   = s.sql_id
AND sq.child_number(+) = s.sql_child_number
AND sq_prev.sql_id(+)   = s.prev_sql_id
AND sq_prev.child_number(+) = s.prev_child_number
AND ( sq.sql_text NOT LIKE '%DD_ACTIVITY_SAMPLING%' OR sq.sql_text is NULL )
AND s.con_id = c.con_id(+)
AND s.command = comm.command_type(+)`

const activityQueryActiveSessionHistory = `SELECT /*+ push_pred(sq) */ /* DD_ACTIVITY_SAMPLING */
	cast (sample_time as date) now,
	(CAST(sample_time_utc AS DATE) - TO_DATE('1970-01-01', 'YYYY-MM-DD')) * 86400000 as utc_ms,
	s.session_id sid,
	s.session_serial# serial#,
	sess.username,
	'ACTIVE' status,
	sess.osuser,
	sess.process,
	s.machine,
	s.port,
	s.program,
	s.session_type type,
	s.sql_id,
	sq.force_matching_signature as force_matching_signature,
	sq.plan_hash_value sql_plan_hash_value,
	s.sql_exec_start,
	s.module,
	s.action,
	sess.logon_time,
	s.client_id client_identifier,
	CASE WHEN s.blocking_session_status = 'VALID' THEN
		s.blocking_inst_id
	ELSE
		null
	END blocking_instance,
	CASE WHEN s.blocking_session_status = 'VALID' THEN
		s.blocking_session
	ELSE
		null
	END blocking_session,
	CASE WHEN session_state = 'WAITING' THEN
		s.event
	ELSE
		'CPU'
	END event,
	CASE WHEN session_state = 'WAITING' THEN
		s.wait_class
	ELSE
		'CPU'
	END wait_class,
	s.wait_time * 10000 wait_time_micro,
	c.name as pdb_name,
	dbms_lob.substr(sq.sql_fulltext, :sql_substr_length, 1) sql_fulltext,
	s.sql_opname command_name
FROM
	v$active_session_history s,
	v$session sess,
	v$sql sq,
	v$containers c
WHERE
	sq.sql_id(+)   = s.sql_id
	AND sq.child_number(+) = s.sql_child_number
	AND ( sq.sql_text NOT LIKE '%DD_ACTIVITY_SAMPLING%' OR sq.sql_text is NULL )
	AND s.con_id = c.con_id(+)
	AND sess.sid(+) = s.session_id AND sess.serial#(+) = s.session_serial#
	AND s.sample_id > :last_sample_id
ORDER BY s.sample_time`
