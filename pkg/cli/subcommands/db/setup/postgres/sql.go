// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package postgres

// SQL constants for the datadog schema, functions, and grants.
// Function bodies mirror integrations-core/postgres/tests/compose/resources/03_setup.sh.
// NOTE: if the canonical SQL in integrations-core changes, this file must be updated manually.
// See the open question in RFC 2026-06-02 about SQL drift.

const sqlCreateExtension = `CREATE EXTENSION IF NOT EXISTS pg_stat_statements`

const sqlCreateSchema = `CREATE SCHEMA IF NOT EXISTS datadog`

const sqlGrantSchemaUsage = `GRANT USAGE ON SCHEMA datadog TO $1`

// sqlFuncPgStatActivity creates a wrapper for pg_stat_activity usable by the datadog user.
const sqlFuncPgStatActivity = `
CREATE OR REPLACE FUNCTION datadog.pg_stat_activity() RETURNS SETOF pg_stat_activity AS
  $$ SELECT * FROM pg_catalog.pg_stat_activity; $$
LANGUAGE sql SECURITY DEFINER`

// sqlFuncPgStatStatements creates a wrapper for pg_stat_statements usable by the datadog user.
const sqlFuncPgStatStatements = `
CREATE OR REPLACE FUNCTION datadog.pg_stat_statements() RETURNS SETOF pg_stat_statements AS
  $$ SELECT * FROM pg_stat_statements; $$
LANGUAGE sql SECURITY DEFINER`

// sqlFuncExplainStatement creates the explain_statement helper.
const sqlFuncExplainStatement = `
CREATE OR REPLACE FUNCTION datadog.explain_statement(
    l_query TEXT,
    OUT explain JSON
)
RETURNS SETOF JSON AS
$$
DECLARE
curs REFCURSOR;
plan JSON;

BEGIN
    OPEN curs FOR EXECUTE pg_catalog.concat('EXPLAIN (FORMAT JSON) ', l_query);
    FETCH curs INTO plan;
    CLOSE curs;
    RETURN QUERY SELECT plan;
END;
$$
LANGUAGE 'plpgsql'
RETURNS NULL ON NULL INPUT
SECURITY DEFINER`

const sqlGrantPGMonitor = `GRANT pg_monitor TO $1`

// sqlGrantPG96 is the fallback for Postgres 9.6 which lacks pg_monitor.
const sqlGrantPG96 = `
GRANT SELECT ON pg_stat_database TO $1;
GRANT SELECT ON pg_stat_database_conflicts TO $1;
GRANT SELECT ON pg_stat_bgwriter TO $1`

const sqlAlterRoleInherit = `ALTER ROLE $1 INHERIT`

const sqlCreateUser = `CREATE USER $1 WITH PASSWORD $2`

const sqlAlterUserPassword = `ALTER USER $1 WITH PASSWORD $2`

const sqlCheckUserExists = `SELECT 1 FROM pg_roles WHERE rolname = $1`

// sqlGetSettings queries the current active and pending-restart state for all
// DBM-relevant server settings.
const sqlGetSettings = `
SELECT name, setting, pending_restart
FROM pg_settings
WHERE name IN (
    'shared_preload_libraries',
    'track_activity_query_size',
    'pg_stat_statements.max',
    'pg_stat_statements.track',
    'track_io_timing',
    'pg_stat_statements.track_utility'
)`

// sqlGetDatabases lists all non-template databases.
const sqlGetDatabases = `
SELECT datname FROM pg_database
WHERE datistemplate = false
ORDER BY datname`

// sqlPGVersion returns the major version as an integer, e.g. 15.
const sqlPGVersion = `SHOW server_version_num`

// sqlCheckRDSSetting detects RDS/Aurora by looking for rds.extensions.
const sqlCheckRDSSetting = `SELECT current_setting('rds.extensions', true)`

// sqlCheckCloudSQLRole detects Cloud SQL by the presence of cloudsqladmin.
const sqlCheckCloudSQLRole = `SELECT 1 FROM pg_roles WHERE rolname = 'cloudsqladmin'`

// sqlVersion returns the full version string (used to distinguish Aurora).
const sqlVersion = `SELECT version()`

// sqlCheckExtensionLoaded checks whether pg_stat_statements is in shared_preload_libraries.
const sqlCheckSPL = `SELECT setting FROM pg_settings WHERE name = 'shared_preload_libraries'`

// sqlReloadConf reloads configuration without a restart.
const sqlReloadConf = `SELECT pg_reload_conf()`

// sqlCheckSuperuser checks whether the connected role has superuser or createrole.
const sqlCheckPrivileges = `SELECT rolsuper, rolcreaterole FROM pg_roles WHERE rolname = current_user`
