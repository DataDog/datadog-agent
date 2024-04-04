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
	dbms_lob.substr(sql_fulltext, 3500, 1) sql_fulltext,
	dbms_lob.substr(prev_sql_fulltext, 3500, 1) prev_sql_fulltext,
	pdb_name,
	command_name
FROM sys.dd_session
WHERE 
	( sql_text NOT LIKE '%DD_ACTIVITY_SAMPLING%' OR sql_text is NULL ) 
	AND (
		NOT (state = 'WAITING' AND wait_class = 'Idle')
		OR state = 'WAITING' AND event = 'fbar timer' AND type = 'USER'
	)
	AND status = 'ACTIVE'`

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
	dbms_lob.substr(sql_fulltext, 3500, 1) sql_fulltext,
	dbms_lob.substr(prev_sql_fulltext, 3500, 1) prev_sql_fulltext,
	command_name
FROM sys.dd_session
WHERE 
	( sql_text NOT LIKE '%DD_ACTIVITY_SAMPLING%' OR sql_text is NULL ) 
	AND (
		NOT (state = 'WAITING' AND wait_class = 'Idle')
		OR state = 'WAITING' AND event = 'fbar timer' AND type = 'USER'
	)
	AND status = 'ACTIVE'`

const activityQueryDirect = `SELECT /*+ push_pred(sq) push_pred(sq_prev) */ /* DD_ACTIVITY_SAMPLING */
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
dbms_lob.substr(sq.sql_fulltext, 3500, 1) sql_fulltext,
dbms_lob.substr(sq_prev.sql_fulltext, 3500, 1) prev_sql_fulltext,
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
AND (
	NOT (state = 'WAITING' AND wait_class = 'Idle')
	OR state = 'WAITING' AND event = 'fbar timer' AND type = 'USER'
)
AND status = 'ACTIVE'
AND s.con_id = c.con_id(+)
AND s.command = comm.command_type(+)`
