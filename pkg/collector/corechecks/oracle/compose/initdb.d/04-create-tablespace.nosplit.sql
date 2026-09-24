declare
  dir dba_data_files.file_name%type;
  l_create_dir v$parameter.value%type;
  tablespace_count number;
begin
  begin
    select value into l_create_dir from v$parameter where name = 'db_create_file_dest';
  exception
    when no_data_found then
      l_create_dir := null;
  end;
  if l_create_dir is null then
    select SUBSTR(file_name,1,(INSTR(file_name,'/',-1,1)-1)) into dir from dba_data_files where rownum = 1;
  end if;
  for tablespace in (
    select 'tbs_test' name, 100 size_mb from dual
    union all
    select 'tbs_test_offline' name, 10 size_mb from dual
  ) loop
    select count(*) into tablespace_count from dba_tablespaces where tablespace_name = upper(tablespace.name);
    if tablespace_count = 0 then
      if l_create_dir is null then
        execute immediate 'create tablespace ' || tablespace.name || ' datafile ''' || dir || '/' || tablespace.name || '01.dbf'' size ' || tablespace.size_mb || 'M';
      else
        execute immediate 'create tablespace ' || tablespace.name || ' datafile size ' || tablespace.size_mb || 'M';
      end if;
    end if;
  end loop;
end;
