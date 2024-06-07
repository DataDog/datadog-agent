-- Description: Create a user for the legacy Oracle integration

@@pkg/collector/corechecks/oracle/sql/lib/init.sql

BEGIN
  IF :connection_type = :connection_type_cdb THEN
    execute immediate 'alter session set container = cdb$root';
    EXECUTE IMMEDIATE 'CREATE USER &&legacy_user IDENTIFIED BY &&password CONTAINER = ALL';
    execute immediate 'GRANT CREATE SESSION TO &&legacy_user CONTAINER=ALL';
  ELSE
    execute immediate 'ALTER SESSION SET "_ORACLE_SCRIPT"=true';
    EXECUTE IMMEDIATE 'CREATE USER &&legacy_user IDENTIFIED BY &&password';
    execute immediate 'GRANT CONNECT TO &&legacy_user';
  END IF;
END;
/
