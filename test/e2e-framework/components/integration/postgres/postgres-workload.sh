#!/bin/bash
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2025-present Datadog, Inc.
#
# Continuous PostgreSQL workload generator for the Datadog postgres integration
# lab. Exercises the normal SQL client surface (SELECT/INSERT/UPDATE/DELETE,
# transactions, aggregates, index scans, sequential scans, VACUUM/ANALYZE) with
# bounded randomness so postgresql.* metrics (transactions, queries,
# bgwriter, locks, table/index stats, pg_stat_statements) stay populated and
# postgres.can_connect reports OK.
set -uo pipefail

PGHOST="${PGHOST:-postgres}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-app_user}"
PGDATABASE="${PGDATABASE:-labdb}"
export PGHOST PGPORT PGUSER PGDATABASE PGPASSWORD

PSQL=(psql --no-psqlrc --quiet --tuples-only --no-align -v ON_ERROR_STOP=0)

echo "[workload] waiting for postgres at ${PGHOST}:${PGPORT}..."
until pg_isready -h "${PGHOST}" -p "${PGPORT}" -U "${PGUSER}" >/dev/null 2>&1; do
  sleep 2
done
echo "[workload] postgres is ready; starting load loop"

iteration=0
while true; do
  iteration=$((iteration + 1))
  acct=$(( (RANDOM % 200) + 1 ))
  amount=$(( (RANDOM % 500) - 250 ))
  owner_suffix=$(( RANDOM % 200 ))

  # A mixed read/write transaction: read balance, write a ledger entry, update
  # the running balance, then read back an aggregate. Wrapped in a transaction
  # so transaction/commit metrics advance.
  "${PSQL[@]}" >/dev/null 2>&1 <<SQL
BEGIN;
SELECT balance FROM accounts WHERE id = ${acct} FOR UPDATE;
INSERT INTO ledger (account_id, amount, note) VALUES (${acct}, ${amount}, 'iter-${iteration}');
UPDATE accounts SET balance = balance + ${amount} WHERE id = ${acct};
COMMIT;
SQL

  # Read-heavy queries to drive index and sequential scans plus aggregates.
  "${PSQL[@]}" >/dev/null 2>&1 <<SQL
SELECT a.owner, count(l.id) AS entries, coalesce(sum(l.amount), 0) AS total
FROM accounts a
LEFT JOIN ledger l ON l.account_id = a.id
WHERE a.owner LIKE 'owner_${owner_suffix}%'
GROUP BY a.owner
ORDER BY total DESC
LIMIT 10;
SELECT count(*) FROM ledger WHERE created_at > now() - interval '5 minutes';
SELECT id, balance FROM accounts ORDER BY balance DESC LIMIT 25;
SQL

  # Occasionally prune old ledger rows and run maintenance so deadtuple/vacuum
  # and table-size metrics move. Bounded so the table does not grow unbounded.
  if (( iteration % 25 == 0 )); then
    "${PSQL[@]}" >/dev/null 2>&1 <<SQL
DELETE FROM ledger WHERE id IN (SELECT id FROM ledger ORDER BY id ASC LIMIT 200);
ANALYZE accounts;
ANALYZE ledger;
SQL
  fi

  if (( iteration % 200 == 0 )); then
    "${PSQL[@]}" >/dev/null 2>&1 <<SQL
VACUUM (ANALYZE) ledger;
SQL
    echo "[workload] completed ${iteration} iterations"
  fi

  # Bounded pacing so the loop is realistic but does not saturate a t3.large.
  sleep_ms=$(( (RANDOM % 400) + 100 ))
  sleep "0.${sleep_ms}"
done
