
prompt create test table
create table t(n number);
grant select,insert on t to c##datadog ;
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
