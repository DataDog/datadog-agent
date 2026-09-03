-- Description: grant privileges to the Datadog user
--
-- container=all is required on every grant below: c##datadog is a common user, but a
-- privilege granted without container=all only applies in CDB$ROOT. CDB_* views are
-- CONTAINERS()-based and silently return rows only for containers where the querying
-- user holds the privilege, so without this the check sees CDB$ROOT and no PDBs --
-- with no error, just missing data. dd_session is the one exception: it is a local
-- view in CDB$ROOT (01-create-function.nosplit.sql), and a common grant on a local
-- object raises ORA-65030.
grant create session to c##datadog container=all;
grant select on v_$session to c##datadog container=all;
grant select on v_$database to c##datadog container=all;
grant select on v_$containers to c##datadog container=all;
grant select on v_$sqlstats to c##datadog container=all;
grant select on v_$instance to c##datadog container=all;
grant select on dba_feature_usage_statistics to c##datadog container=all;
grant select on V_$SQL_PLAN_STATISTICS_ALL to c##datadog container=all;
grant select on V_$PROCESS to c##datadog container=all;
grant select on V_$SESSION to c##datadog container=all;
grant select on V_$CON_SYSMETRIC to c##datadog container=all;
grant select on CDB_TABLESPACE_USAGE_METRICS to c##datadog container=all;
grant select on CDB_TABLESPACES to c##datadog container=all;
grant select on V_$SQLCOMMAND to c##datadog container=all;
grant select on V_$DATAFILE to c##datadog container=all;
grant select on V_$SYSMETRIC to c##datadog container=all;
grant select on V_$SGAINFO to c##datadog container=all;
grant select on V_$PDBS to c##datadog container=all;
grant select on CDB_SERVICES to c##datadog container=all;
grant select on V_$OSSTAT to c##datadog container=all;
grant select on V_$PARAMETER to c##datadog container=all;
grant select on V_$SQLSTATS to c##datadog container=all;
grant select on V_$CONTAINERS to c##datadog container=all;
grant select on V_$SQL_PLAN_STATISTICS_ALL to c##datadog container=all;
grant select on V_$SQL to c##datadog container=all;
grant select on V_$PGASTAT to c##datadog container=all;
grant select on v_$asm_diskgroup to c##datadog container=all;
grant select on v_$rsrcmgrmetric to c##datadog container=all;
grant select on v_$dataguard_config to c##datadog container=all;
grant select on v_$dataguard_stats to c##datadog container=all;
grant select on v_$transaction to c##datadog container=all;
grant select on v_$locked_object to c##datadog container=all;
grant select on v_$lock to c##datadog container=all;
grant select on gv_$lock to c##datadog container=all;
grant select on dba_objects to c##datadog container=all;
grant select on cdb_data_files to c##datadog container=all;
grant select on dba_data_files to c##datadog container=all;

GRANT SELECT ON dd_session TO c##datadog;

-- Schema collection reads the CDB_* dictionary views. DBA_* views only ever show the
-- container the session is connected to, so the CDB_ variants are required even on a
-- single-PDB test database.
grant select on cdb_users to c##datadog container=all;
grant select on cdb_objects to c##datadog container=all;
grant select on cdb_tables to c##datadog container=all;
grant select on cdb_object_tables to c##datadog container=all;
grant select on cdb_tab_cols to c##datadog container=all;
grant select on cdb_tab_comments to c##datadog container=all;
grant select on cdb_col_comments to c##datadog container=all;
grant select on cdb_indexes to c##datadog container=all;
grant select on cdb_ind_columns to c##datadog container=all;
grant select on cdb_constraints to c##datadog container=all;
grant select on cdb_cons_columns to c##datadog container=all;
grant select on cdb_part_tables to c##datadog container=all;
grant select on cdb_part_key_columns to c##datadog container=all;
grant select on cdb_tab_modifications to c##datadog container=all;
grant select on cdb_external_tables to c##datadog container=all;
grant select on cdb_external_locations to c##datadog container=all;
grant select on cdb_mviews to c##datadog container=all;
grant select on cdb_views to c##datadog container=all;

-- Blockchain and immutable tables are 21c+, so these views may be absent depending on
-- the image pinned by DBMS_VERSION.
declare
  table_or_view_does_not_exist exception;
  pragma exception_init(table_or_view_does_not_exist, -942);
begin
  for v in (select column_value name from table(sys.odcivarchar2list(
              'cdb_blockchain_tables', 'cdb_immutable_tables'))) loop
    begin
      execute immediate 'grant select on ' || v.name || ' to c##datadog container=all';
    exception
      when table_or_view_does_not_exist then null;
    end;
  end loop;
end;
/
