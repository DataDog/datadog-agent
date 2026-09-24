declare
  user_count number;
begin
  select count(*) into user_count from dba_users where username = 'C##DATADOG';
  if user_count = 0 then
    execute immediate 'CREATE USER c##datadog IDENTIFIED BY datadog CONTAINER=ALL';
  end if;
  execute immediate 'ALTER USER c##datadog SET CONTAINER_DATA=ALL CONTAINER=CURRENT';
end;
