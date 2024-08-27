-- Description: grant privileges to the legacy Datadog user

@@pkg/collector/corechecks/oracle/sql/lib/init.sql

begin
  if :connection_type = :connection_type_cdb then
    execute immediate 'GRANT CREATE SESSION TO &&legacy_user CONTAINER=ALL';
    execute immediate 'Grant select any dictionary to &&legacy_user container=all';
  else
    declare
      type array_t is table of varchar2(30);
      array array_t := array_t(
        'GV_$PROCESS',
        'GV_$SYSMETRIC',
        'dba_data_files',
        'dba_tablespaces',
        'dba_tablespace_usage_metrics'
      );
      command varchar2(4000);
      object_name varchar2(30);
    begin
      for i in 1..array.count loop
        if :hostingType = :hostingTypeSelfManaged then
          command := 'grant select on ' || array(i) || ' to &&legacy_user with grant option';
        elsif :hostingType = :hostingTypeRDS then
          command := 'begin rdsadmin.rdsadmin_util.grant_sys_object(''' || upper(array(i)) || ''',''&&legacy_user'',''SELECT'', p_grant_option => false); end;';
        elsif :hostingType = :hostingTypeOCI then
          object_name := replace(array(i), 'GV_$', 'GV$');
          command := 'grant select on ' || array(i) || ' to &&legacy_user with grant option';
        end if;
        begin
          dbms_output.disable;
          execute immediate command;
          dbms_output.enable;
        exception
          when others then
            null;
        end;
      end loop;
    end;
  end if;
end;
/
