-- These views are 21c+ and absent on older versions. The .nosplit.sql suffix keeps
-- TestMain from executing this PL/SQL block line-by-line.
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
