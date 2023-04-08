set verify off

accept password hide prompt password:


CREATE USER c##datadog IDENTIFIED BY &password CONTAINER = ALL ;

ALTER USER c##datadog SET CONTAINER_DATA=ALL CONTAINER=CURRENT;

grant create session to c##datadog ;
grant select on v_$session to c##datadog ;
grant select on v_$database to c##datadog ;
grant select on v_$containers to c##datadog;
grant select on v_$sqlstats to c##datadog ;
grant select on v_$instance to c##datadog ;
grant select on dba_feature_usage_statistics to c##datadog ;

CREATE OR REPLACE VIEW dd_session AS
SELECT 
    s.indx as sid,
    s.ksuseser as serial#,
    s.ksuudlna as username,
    DECODE(BITAND(s.ksuseidl, 9), 1, 'ACTIVE', 0, DECODE(BITAND(s.ksuseflg, 4096), 0, 'INACTIVE', 'CACHED'), 'KILLED') as status,
    s.ksuseunm as osuser,
    s.ksusepid as process,
    s.ksusemnm as machine,
    s.ksusemnp as port,
    s.ksusepnm as program,
    DECODE(BITAND(s.ksuseflg, 19), 17, 'BACKGROUND', 1, 'USER', 2, 'RECURSIVE', '?') as type,
    s.ksusesqi as sql_id,
    sq.force_matching_signature as force_matching_signature,
    s.ksusesph as sql_plan_hash_value,
    s.ksusesesta as sql_exec_start,
    s.ksusepsi as prev_sql_id,
    s.ksusepha as prev_sql_plan_hash_value,
    s.ksusepesta as prev_sql_exec_start,
    sq_prev.force_matching_signature as prev_force_matching_signature,
    s.ksuseapp as module,
    s.ksuseact as action,
    s.ksusecli as client_info,
    s.ksuseltm as logon_time,
    s.ksuseclid as client_identifier,
    decode(s.ksuseblocker, 
        4294967295, 'UNKNOWN', 4294967294, 'UNKNOWN', 4294967293, 'UNKNOWN', 4294967292, 'NO HOLDER', 4294967291, 'NOT IN WAIT', 
        'VALID'
    ) as blocking_session_status,
    DECODE(s.ksuseblocker, 
        4294967295, TO_NUMBER(NULL), 4294967294, TO_NUMBER(NULL), 4294967293, TO_NUMBER(NULL), 
        4294967292, TO_NUMBER(NULL), 4294967291, TO_NUMBER(NULL), BITAND(s.ksuseblocker, 2147418112) / 65536
    ) as blocking_instance,
    DECODE(s.ksuseblocker, 
        4294967295, TO_NUMBER(NULL), 4294967294, TO_NUMBER(NULL), 4294967293, TO_NUMBER(NULL), 
        4294967292, TO_NUMBER(NULL), 4294967291, TO_NUMBER(NULL), BITAND(s.ksuseblocker, 65535)
    ) as blocking_session,
    DECODE(s.ksusefblocker, 
        4294967295, 'UNKNOWN', 4294967294, 'UNKNOWN', 4294967293, 'UNKNOWN', 4294967292, 'NO HOLDER', 4294967291, 'NOT IN WAIT', 'VALID'
    ) as final_blocking_session_status,
    DECODE(s.ksusefblocker, 
        4294967295, TO_NUMBER(NULL), 4294967294, TO_NUMBER(NULL), 4294967293, TO_NUMBER(NULL), 4294967292, TO_NUMBER(NULL), 
        4294967291, TO_NUMBER(NULL), BITAND(s.ksusefblocker, 2147418112) / 65536
    ) as final_blocking_instance,
    DECODE(s.ksusefblocker, 
        4294967295, TO_NUMBER(NULL), 4294967294, TO_NUMBER(NULL), 4294967293, TO_NUMBER(NULL), 4294967292, TO_NUMBER(NULL), 
        4294967291, TO_NUMBER(NULL), BITAND(s.ksusefblocker, 65535)
    ) as final_blocking_session,
    DECODE(w.kslwtinwait, 
        1, 'WAITING', decode(bitand(w.kslwtflags, 256), 0, 'WAITED UNKNOWN TIME', 
        decode(round(w.kslwtstime / 10000), 0, 'WAITED SHORT TIME', 'WAITED KNOWN TIME'))
    ) as STATE,
    e.kslednam as event,
    e.ksledclass as wait_class,
    w.kslwtstime as wait_time_micro,
    c.name as pdb_name,
    sq.sql_text as sql_text,
    sq.sql_fulltext as sql_fulltext,
    prev_sq.sql_fulltext as prev_sql_fulltex /*,
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
    sq.avoided_executions,*/
    comm.command_name
  FROM
    x$ksuse s,
    x$kslwt w, 
    x$ksled e,
    v$sqlstats sq,
    v$sqlstats sq_prev,
    v$containers c,
    v$sqlcommand comm
  WHERE
    BITAND(s.ksspaflg, 1) != 0
    AND BITAND(s.ksuseflg, 1) != 0
    AND s.inst_id = USERENV('Instance')
    AND s.indx = w.kslwtsid
    AND w.kslwtevt = e.indx
    AND s.ksusesqi = sq.sql_id(+)
    AND s.ksusesph = sq.plan_hash_value(+)
    AND s.ksusepsi = sq_prev.sql_id(+)
    AND s.ksusepha = sq_prev.plan_hash_value(+)
    AND s.con_id = c.con_id(+)
    AND s.ksuudoct = comm.command_type
;

GRANT SELECT ON dd_session TO c##datadog ;

-- for compatibility with the existing Oracle integration
GRANT CREATE SESSION TO c##datadog CONTAINER=ALL;
Grant select any dictionary to c##datadog container=all;
GRANT SELECT ON GV_$PROCESS TO c##datadog CONTAINER=ALL;
GRANT SELECT ON gv_$sysmetric TO c##datadog CONTAINER=ALL;
