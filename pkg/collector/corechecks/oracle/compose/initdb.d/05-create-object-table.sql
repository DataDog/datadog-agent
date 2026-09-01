CREATE OR REPLACE TYPE dd_address_t AS OBJECT (street VARCHAR2(100), city VARCHAR2(100));
CREATE TABLE dd_addresses OF dd_address_t;
GRANT SELECT ON dd_addresses TO c##datadog;
