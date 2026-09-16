-- Unless explicitly stated otherwise all files in this repository are licensed
-- under the Apache License Version 2.0.
-- This product includes software developed at Datadog (https://www.datadoghq.com/).
-- Copyright 2025-present Datadog, Inc.
--
-- Seed script run once by the postgres:16 entrypoint against labdb.
-- Creates:
--   * the `datadog` monitoring user (granted pg_monitor) used by the Agent check
--   * the pg_stat_statements extension and the monitoring helper schema/functions
--     recommended by the Datadog postgres integration
--   * an `app_user` plus a small seeded schema for the workload generator

-- Monitoring user for the Datadog Agent. Password matches config/postgres.yaml.
CREATE USER datadog WITH PASSWORD 'datadog_monitor_pw';
GRANT pg_monitor TO datadog;
GRANT SELECT ON pg_stat_database TO datadog;

-- pg_stat_statements lives in the database the check connects to.
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- Datadog explain helper schema (used by DBM; harmless without it).
CREATE SCHEMA IF NOT EXISTS datadog;
GRANT USAGE ON SCHEMA datadog TO datadog;
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
SECURITY DEFINER;

-- Application user and seeded data for the workload generator.
CREATE USER app_user WITH PASSWORD 'app_user_pw';

CREATE TABLE IF NOT EXISTS accounts (
    id          BIGSERIAL PRIMARY KEY,
    owner       TEXT NOT NULL,
    balance     NUMERIC(14, 2) NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ledger (
    id          BIGSERIAL PRIMARY KEY,
    account_id  BIGINT NOT NULL REFERENCES accounts(id),
    amount      NUMERIC(14, 2) NOT NULL,
    note        TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ledger_account_idx ON ledger(account_id);
CREATE INDEX IF NOT EXISTS accounts_owner_idx ON accounts(owner);

INSERT INTO accounts (owner, balance)
SELECT 'owner_' || g, (random() * 10000)::numeric(14, 2)
FROM generate_series(1, 200) AS g;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO app_user;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_user;
