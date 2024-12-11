-- Description: Setup test environment for Oracle tests

@@pkg/collector/corechecks/oracle/sql/lib/init.sql

var username varchar2(30);
var legacy_username varchar2(30);
BEGIN
  IF :connection_type = 'CDB' THEN
    :username := 'C##DATADOG';
    :legacy_username := 'C##DATADOG_LEGACY';
  ELSE
    :username := 'DATADOG';
    :legacy_username := 'DATADOG_LEGACY';
  END IF;
END;
/

set term off
column username new_value user
column legacy_username new_value legacy_user
column password new_value password
select :username username, :legacy_username legacy_username, 'datadog' password from dual;
set term on

prompt create user
@@pkg/collector/corechecks/oracle/sql/user/create_user.sql
prompt grants
@@pkg/collector/corechecks/oracle/sql/user/grants.sql
prompt activity view
@@pkg/collector/corechecks/oracle/sql/user/create_activity_view.sql

prompt create legacy user
@@pkg/collector/corechecks/oracle/sql/user/legacy/create_user.sql
prompt create legacy user grants
@@pkg/collector/corechecks/oracle/sql/user/legacy/grants.sql

prompt create test table
create table t(n number);
grant select,insert on t to &&user ;
insert into t values(18446744073709551615);
commit;

prompt create test tablespace
declare
  dir dba_data_files.file_name%type;
  l_create_dir v$parameter.value%type;
begin
  begin
    select value into l_create_dir from v$parameter where name = 'db_create_file_dest';
  exception
    when no_data_found then
      l_create_dir := null;
  end;
  if l_create_dir is null then
    select SUBSTR(file_name,1,(INSTR(file_name,'/',-1,1)-1)) into dir from dba_data_files where rownum = 1;
    execute immediate 'create tablespace tbs_test datafile ''' || dir || '/tbs_test01.dbf'' size 100M';
    execute immediate 'create tablespace tbs_test_offline datafile ''' || dir || '/tbs_test_offline01.dbf'' size 10M';
  else
    execute immediate 'create tablespace tbs_test datafile size 100M';
    execute immediate 'create tablespace tbs_test_offline datafile size 10M';
  end if;
end;
/
