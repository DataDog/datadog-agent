-- Description: grant privileges to the Datadog user
set serveroutput on size 100000

@@pkg/collector/corechecks/oracle/sql/lib/init.sql

grant create session to &&user ;

declare
   type array_t is table of varchar2(30);
   monitoring_views array_t := array_t(
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
    'dba_data_files'
   );
   schema_views array_t := array_t(
    'cdb_users',
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
    'cdb_external_tables',
    'cdb_external_locations',
    'cdb_mviews',
    'cdb_views',
    'cdb_blockchain_tables',
    'cdb_immutable_tables'
   );
   container_clause varchar2(20);

   procedure grant_views(view_names array_t, privilege_name varchar2, scope_clause varchar2 default null) is
      command varchar2(4000);
   begin
      for i in 1..view_names.count loop
         if :hostingType = :hostingTypeSelfManaged then
            command := 'grant ' || privilege_name || ' on ' || view_names(i) || ' to &&user' || scope_clause;
         elsif :hostingType = :hostingTypeRDS then
            command := 'begin rdsadmin.rdsadmin_util.grant_sys_object(''' || upper(view_names(i)) || ''',''&&user'',''SELECT'', p_grant_option => false); end;';
         elsif :hostingType = :hostingTypeOCI then
            command := 'grant select on ' || view_names(i) || ' to &&user with grant option' || scope_clause;
         end if;
         begin
            execute immediate command;
         exception
            when others then
               null;
         end;
      end loop;
   end;
begin
   if :connection_type = :connection_type_cdb then
      container_clause := ' container=all';
      begin
         execute immediate 'grant set container to &&user container=all';
      exception
         when others then
            dbms_output.put_line('Could not grant SET CONTAINER to &&user: ' || sqlerrm);
      end;
   end if;

   grant_views(monitoring_views, 'select');
   if dbms_db_version.version > 12 or (dbms_db_version.version = 12 and dbms_db_version.release >= 2) then
      grant_views(schema_views, 'read');
      if dbms_db_version.version < 23 then
         grant_views(array_t('dba_tab_cols'), 'read', container_clause);
      end if;
   end if;
end;
/
