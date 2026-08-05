#!/usr/bin/env bash
set -euo pipefail

: "${POSTGRES_USER:=bob}"
: "${EXPECTED_AGENT_HOSTNAME:?EXPECTED_AGENT_HOSTNAME is required}"
: "${RQ_POSTGRES_HOST:?RQ_POSTGRES_HOST is required}"
: "${RQ_POSTGRES_TARGET_PORT:?RQ_POSTGRES_TARGET_PORT is required}"
: "${RQ_DB_PREFIX:?RQ_DB_PREFIX is required}"

for suffix in db1 db2; do
  dbname="${RQ_DB_PREFIX}_${suffix}"
  marker="${EXPECTED_AGENT_HOSTNAME}|${RQ_POSTGRES_HOST}|${RQ_POSTGRES_TARGET_PORT}|${dbname}"

  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres <<SQL
CREATE DATABASE ${dbname};
SQL

  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$dbname" <<SQL
CREATE TABLE remote_query_identity (
  expected_agent_hostname text NOT NULL,
  expected_postgres_host text NOT NULL,
  expected_postgres_port integer NOT NULL,
  expected_dbname text NOT NULL,
  marker text NOT NULL
);
INSERT INTO remote_query_identity (
  expected_agent_hostname,
  expected_postgres_host,
  expected_postgres_port,
  expected_dbname,
  marker
) VALUES (
  '${EXPECTED_AGENT_HOSTNAME}',
  '${RQ_POSTGRES_HOST}',
  ${RQ_POSTGRES_TARGET_PORT},
  '${dbname}',
  '${marker}'
);

CREATE TABLE cities (city text NOT NULL, country text NOT NULL);
INSERT INTO cities(city, country) VALUES
  ('Beautiful city of lights', 'France'),
  ('New York', 'USA');
SQL
done
