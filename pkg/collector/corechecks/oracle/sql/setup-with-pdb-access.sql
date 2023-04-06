set verify off

accept password hide prompt password:

define user = "datadog"

create user &user identified by &password ;
grant create session to &user ;
grant select on v_$session to &user ;
grant select on v_$database to &user ;
grant select on v_$containers to &user ;
grant select on v_$sqlstats to &user ;
grant select on v_$instance to &user ;
grant select on dba_feature_usage_statistics to &user ;

CREATE OR REPLACE VIEW dd_session AS
SELECT 
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
    sql_exec_start,
    module,
    action,
    client_info,
    logon_time,
    client_identifier,
    blocking_session_status,
    blocking_instance,
    blocking_session,
    final_blocking_session_status,
    final_blocking_instance,
    final_blocking_session,
    state,
    event,
    wait_class,
    wait_time_micro,
    c.name as pdb_name,
    sq.sql_text as sql_text,
    sq.sql_fulltext as sql_fulltext,
    sq.parse_calls,
    sq.disk_reads,
    sq.direct_writes,
    sq.direct_reads,
    sq.buffer_gets,
    sq.rows_processed,
    sq.serializable_aborts,
    sq.fetches,
    sq.executions,
    sq.end_of_fetch_count,
    sq.loads,
    sq.version_count,
    sq.invalidations,
    sq.px_servers_executions,
    sq.cpu_time,
    sq.elapsed_time,
    sq.avg_hard_parse_time,
    sq.application_wait_time,
    sq.concurrency_wait_time,
    sq.cluster_wait_time,
    sq.user_io_wait_time,
    sq.plsql_exec_time,
    sq.java_exec_time,
    sq.sorts,
    sq.sharable_mem,
    sq.typecheck_mem,
    sq.io_cell_offload_eligible_bytes,
    sq.io_interconnect_bytes,
    sq.physical_read_requests,
    sq.physical_read_bytes,
    sq.physical_write_requests,
    sq.physical_write_bytes,
    sq.io_cell_uncompressed_bytes,
    sq.io_cell_offload_returned_bytes,
    sq.avoided_executions
  FROM
    v$session s,
    v$sqlstats sq,
    v$containers c
  WHERE
    status = 'ACTIVE'
    AND s.inst_id = USERENV('Instance')
    AND s.sql_id = sq.sql_id(+)
    AND s.con_id = c.con_id(+)
;

GRANT SELECT ON dd_session TO &user ;

-- for compatibility with the existing Oracle integration
GRANT CREATE SESSION TO &user ;
Grant select any dictionary to &user ;
GRANT SELECT ON GV_$PROCESS TO &user ;
GRANT SELECT ON gv_$sysmetric TO &user ;
