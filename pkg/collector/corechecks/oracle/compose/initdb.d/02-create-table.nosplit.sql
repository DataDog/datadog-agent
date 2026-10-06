declare
  table_count number;
begin
  select count(*) into table_count from user_tables where table_name = 'T';
  if table_count = 0 then
    execute immediate 'create table t(n number)';
  end if;
  execute immediate 'grant select,insert on t to c##datadog';
  execute immediate 'insert into t select 18446744073709551615 from dual where not exists (select 1 from t where n = 18446744073709551615)';
end;
