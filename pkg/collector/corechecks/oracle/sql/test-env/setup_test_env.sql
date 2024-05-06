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
create table sys.t(n number);
grant select,insert on sys.t to &&user ;
insert into sys.t values(18446744073709551615);
commit;

prompt create test tablespace
declare
  dir varchar2(1000);
begin
  select SUBSTR(file_name,1,(INSTR(file_name,'/',-1,1)-1)) into dir from dba_data_files where rownum = 1;
  execute immediate 'create tablespace tbs_test datafile ''' || dir || '/tbs_test01.dbf'' size 100M';
end;
/

