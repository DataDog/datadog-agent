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
    'dba_objects',
    'cdb_data_files',
    'dba_data_files'
  );
  command varchar2(100);
  object_name varchar2(30);
begin
   for i in 1..array.count loop
      if :hostingType = :hostingTypeSelfManaged then
        command := 'grant select on ' || array(i) || ' to &&user';
      elsif :hostingType = :hostingTypeRDS then
        command := 'rdsadmin.rdsadmin_util.grant_sys_object(''' || array(i) || ',''&&user'',''SELECT'', p_grant_option => false)';
      elsif :hostingType = :hostingTypeOCI then
        object_name := replace(array(i), 'V_$', 'V$');
        command := 'grant select on ' || array(i) || ' to &&user with grant option';
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
