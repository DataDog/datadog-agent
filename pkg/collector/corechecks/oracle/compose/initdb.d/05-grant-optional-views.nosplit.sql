-- Description: grant privileges on dictionary views that only exist on newer databases
--
-- Blockchain and immutable tables are 21c+; these views are absent on older databases.
-- This must stay in a .nosplit.sql file: TestMain executes every other initdb.d file
-- line-by-line, which would shred a PL/SQL block into invalid fragments.
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
