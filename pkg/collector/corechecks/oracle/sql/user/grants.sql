-- Description: grant privileges to the Datadog user
set serveroutput on size 100000

@@pkg/collector/corechecks/oracle/sql/lib/init.sql

grant create session to &&user ;

declare
   type array_t is table of varchar2(30);
   array array_t := array_t(
    'v_$session',
    'v_$database',
    'v_$containers',
    'v_$sqlstats',
    'v_$instance',
    'dba_feature_usage_statistics',
    'V_$SQL_PLAN_STATISTICS_ALL',
    'V_$PROCESS',
    'V_$SESSION',
    'V_$CON_SYSMETRIC',
    'CDB_TABLESPACE_USAGE_METRICS',
    'CDB_TABLESPACES',
    'V_$SQLCOMMAND',
    'V_$DATAFILE',
    'V_$SYSMETRIC',
    'V_$SGAINFO',
    'V_$PDBS',
    'CDB_SERVICES',
    'V_$OSSTAT',
    'V_$PARAMETER',
    'V_$SQLSTATS',
    'V_$CONTAINERS',
    'V_$SQL_PLAN_STATISTICS_ALL',
    'V_$SQL',
    'V_$PGASTAT',
    'v_$asm_diskgroup',
    'v_$rsrcmgrmetric',
    'v_$dataguard_config',
    'v_$dataguard_stats',
    'v_$transaction',
    'v_$locked_object',
    'v_$active_session_history',
    'v_$lock',
    'gv_$lock',
    'dba_objects',
    'cdb_data_files',
    'dba_data_files',
    'cdb_users',
    'cdb_objects',
    'cdb_tables',
    'cdb_object_tables',
    'cdb_tab_cols',
    'cdb_tab_comments',
    'cdb_col_comments',
    'cdb_indexes',
    'cdb_ind_columns',
    'cdb_constraints',
    'cdb_cons_columns',
    'cdb_part_tables',
    'cdb_part_key_columns',
    'cdb_tab_modifications',
    'cdb_external_tables',
    'cdb_external_locations',
    'cdb_mviews',
    'cdb_views',
    'cdb_blockchain_tables',
    'cdb_immutable_tables'
  );
  command varchar2(4000);
  object_name varchar2(30);
  -- CDB_* views are CONTAINERS()-based: a grant without container=all only applies in
  -- CDB$ROOT, so the query silently sees root's rows only in every PDB, with no error.
  -- RDS's grant_sys_object has no container=all equivalent, but that is moot there --
  -- RDS master users are always local, so connection_type is never CDB on RDS.
  container_clause varchar2(20) := '';
begin
   if :connection_type = :connection_type_cdb then
      container_clause := ' container=all';
   end if;
   for i in 1..array.count loop
      if :hostingType = :hostingTypeSelfManaged then
        command := 'grant select on ' || array(i) || ' to &&user' || container_clause;
      elsif :hostingType = :hostingTypeRDS then
        command := 'begin rdsadmin.rdsadmin_util.grant_sys_object(''' || upper(array(i)) || ''',''&&user'',''SELECT'', p_grant_option => false); end;';
      elsif :hostingType = :hostingTypeOCI then
        object_name := replace(array(i), 'V_$', 'V$');
        command := 'grant select on ' || array(i) || ' to &&user with grant option' || container_clause;
      end if;
      begin
         execute immediate command;
      exception
         when others then
            null;
      end;
   end loop;
end;
/
