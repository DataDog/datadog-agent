-- Description: Setup test environment for Oracle tests

@@../lib/init.sql

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
@@../user/create_user.sql
prompt grants
@@../user/grants.sql
prompt activity view
@@../user/create_activity_view.sql

prompt create legacy user
@@../user/legacy/create_user.sql
prompt create legacy user grants
@@../user/legacy/grants.sql

prompt create test table
create table sys.t(n number);
grant insert on sys.t to &&user ;

prompt create test tablespace
declare
  dir varchar2(1000);
begin
  select SUBSTR(file_name,1,(INSTR(file_name,'/',-1,1)-1)) into dir from dba_data_files where rownum = 1;
  execute immediate 'create tablespace tbs_test datafile ''' || dir || '/tbs_test01.dbf'' size 100M';
end;
/

