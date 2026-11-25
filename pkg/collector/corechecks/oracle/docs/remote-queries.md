# Oracle remote queries (proof of concept)

The native Oracle check implements `check.RemoteQuerier` (`remote_query*.go`, build tag `oracle`).
The core Agent's [remote queries specification](../../../remotequeries/docs/spec.md) defines the
`resolve` and `execute` calls; this note records what the Oracle integration adds.

## Scope

- **Target:** the check's own configured direct server only: `server` (no `host:port` shorthand),
  separate `port` (default 1521), explicit `service_name`, TCP, `go-ora`. Instances using
  `tns_alias`, `tns_admin`, `oracle_client`/`instant_client`, TCPS, a wallet, or no service name
  answer `target_not_found`, so they never make another Oracle instance's resolution ambiguous.
  `database_instance` targets are not served. Host matching trims, lowercases and removes one
  trailing dot; the service name is trimmed and compared case-insensitively.
- **Resolve** reads configuration only: no connection, no SQL. **Execute** checks the target again.
- **Query:** one fixed fixture statement, never caller SQL. The caller's query must equal it after
  whitespace normalisation, otherwise the run fails with `unsupported_query` before connecting:

  ```sql
  SELECT ID, LABEL FROM RQ_ORACLE_PROBE ORDER BY ID, LABEL
  ```

  `RQ_ORACLE_PROBE` is `(ID NUMBER, LABEL VARCHAR2(100))` in the configured login's schema.
- **Connection:** a separate go-ora connection per run with the check's configured login (use a
  least-privilege user with `SELECT` on the fixture). It never uses the monitoring connection or
  state. The transaction starts with `SET TRANSACTION READ ONLY` because go-ora rejects
  `sql.TxOptions{ReadOnly: true}`; the fixed statement is the primary guard.
- **Types:** `NUMBER` (logical `decimal`, exact driver text, never a float) and `VARCHAR2`. Every
  other type, including LOBs, dates and binary, fails with `unsupported_column`.
- **Upload:** the JSON-cell session (`csv-json-cell-v1`): descriptor, bounded pages, finalize on
  success, abort on any other end. The destination is derived from the Agent's `site` and the
  request is authenticated with the Agent's API key alone; redirects are never followed.
  Pages are sent uncompressed.
- **Lifecycle:** `Cancel` (unscheduling) refuses new calls, cancels runs, closes their sockets and
  waits for each run's abort request. Closing the socket stops the Agent's side of the run; it is
  not proof that Oracle stops the statement.
- **Logs and errors** never contain SQL, credentials, DSNs, hosts or rows. A database failure
  carries at most its `ORA-nnnnn` code.

## Live smoke test

`remote_query_live_test.go` runs the real `openRemoteOracle`/go-ora path against a real Oracle
database with a local HTTPS stub intake. It is skipped unless `RQ_ORACLE_LIVE_HOST` is set (see the
file header for the login variables). It was run against Oracle Free 23.26 (`FREEPDB1`, ARM64) with
a SELECT-only login that reaches `RQ_ORACLE_PROBE` through a private synonym. It checks exact
`NUMBER` text beyond 2^53 and with a 37-digit fraction, `VARCHAR2` with Unicode and quoting, NULLs,
page upload and finalize, an empty result, and that the run's transaction refuses a write
(`ORA-01456`) for a login that may write. It does not prove backend cancellation.
