CREATE OR REPLACE VIEW dd_session AS
SELECT 
    s.indx as sid,
    s.ksuseser as serial#,
    s.ksuudlna as username,
    s.ksuseunm as osuser,
    s.ksusepid as process,
    s.ksusemnm as machine,
    s.ksusepnm as program,
    DECODE(BITAND(s.ksuseflg, 19), 17, 'BACKGROUND', 1, 'USER', 2, 'RECURSIVE', '?') as type,
    s.ksusesqi as sql_id,
    sq.force_matching_signature as force_matching_signature,
    s.ksusesph as sql_plan_hash_value,
    s.ksusesesta as sql_exec_start,
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
    sq.sql_text as sql_text,
    c.name as pdb_name
  FROM
    x$ksuse s,
    x$kslwt w,
    x$ksled e,
    v$sqlstats sq,
    v$containers c
  WHERE
    BITAND(s.ksspaflg, 1) != 0
    AND BITAND(s.ksuseflg, 1) != 0
    AND s.inst_id = USERENV('Instance')
    AND s.indx = w.kslwtsid
    AND w.kslwtevt = e.indx
    AND s.ksusesqi = sq.sql_id(+)
    AND s.con_id = c.con_id(+)
    AND BITAND(s.ksuseidl, 11) = 1 --ACTIVE
;

GRANT SELECT ON dd_session TO c##datadog ;
