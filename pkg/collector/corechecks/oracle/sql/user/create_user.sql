-- Description: Create Datadog Agent user

@@pkg/collector/corechecks/oracle/sql/lib/init.sql

BEGIN
  IF :connection_type = :connection_type_cdb THEN
    EXECUTE IMMEDIATE 'CREATE USER &&user IDENTIFIED BY &&password CONTAINER = ALL';
    EXECUTE IMMEDIATE 'ALTER USER &&user SET CONTAINER_DATA=ALL CONTAINER=CURRENT';
  ELSE
    EXECUTE IMMEDIATE 'CREATE USER &&user IDENTIFIED BY &&password';
  END IF;
END;
/
