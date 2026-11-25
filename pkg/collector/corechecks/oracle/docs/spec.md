# Oracle integration remote queries: specification

## Purpose

The Oracle Agent integration runs Remote Queries for its loaded checks. When the Agent asks, a check says whether it serves the requested database. When the Agent hands it a query, the check runs the query in a read-only transaction on a connection of its own, encodes each row in intake's JSON-cell upload format, and uploads the rows, split into pages, to `its-agent-intake`, which converts the pages into JSON result files. The check then finalizes the upload, or aborts it when the run fails, and answers the Agent with success or an error.

The integration does not choose which Agent or check runs a query (the [Agent](https://dd-quick.us1.staging.dog/datadog-agent-spec/) and the `its-agent` worker do), validate SQL beyond its statement kind (QUERY-02), write result files or scan for sensitive data (intake does), or report run status to ITS (the worker does). Its only caller is the core Agent.

## Terms

- **Agent:** the core Datadog Agent, which calls the integration (Agent spec, Terms).
- **Check:** one loaded Oracle check, built from one instance configuration.
- **Target, delivery instructions, run:** as defined in the [Agent spec](https://dd-quick.us1.staging.dog/datadog-agent-spec/).
- **Intake:** `its-agent-intake`, which receives pages and stores the result files.
- **Upload session, descriptor, page, result file, page index, record offset:** as defined in the [intake spec](https://dd-quick.us1.staging.dog/its-agent-intake-spec/).
- **Direct connection:** a check configuration that the check connects to with the go-ora driver: it has no `tns_alias`, and `oracle_client` is off. It uses `server`, `port`, and `service_name`, with TCPS and a wallet when `protocol` is `TCPS`.
- **Cell:** one field of a page: exactly one JSON value (intake INPUT-01).
- **Result file size target:** the result file size in the delivery instructions, capped at the page memory limit. The integration aims for it when it builds pages (SIZE-01), on a best-effort basis; intake, not the integration, enforces any hard cap.
- **Row size limit, page count limit, total result size limit, column limit, schema size limit:** the limits in the delivery instructions, which intake returned when it created the session.
- **Run time limit:** the query time limit in the delivery instructions (its-agent TIME-03).
- **Database time:** the part of a run spent starting the query and reading its rows.

## Receiving a request

### REQ-01 — Serve two operations

The integration serves two operations: resolve and execute (Agent annex). A request for any other operation, or one that is not one JSON object, fails as an invalid request.

### REQ-02 — Validate a request before using it

A match check carries only the operation and the target. A query request carries the operation, the target, a non-empty query, the schema choice, the delivery instructions, and optionally the trace context. The target and the delivery instructions follow the Agent's rules (Agent INPUT-03, INPUT-04). The upload address has an `https` or `http` scheme and a host, and no credentials, query, fragment, spaces, or control characters. A request with an unknown or repeated field, or a missing or invalid value, fails as an invalid request, without contacting the database or intake.

### REQ-03 — Require the Agent's API key

When the Agent has no API key configured, a query request fails before connecting to the database.

## Matching the target

### MATCH-01 — Match a host, port, and service target

A check serves a host, port, and database target when its configuration is a direct connection and all of these hold:

- its configured server equals the target's host, both trimmed of surrounding spaces, lowercased, and without one trailing dot;
- its port equals the target's port; a `server` written as `host:port` gives that port when `port` is not set;
- its configured service name equals the target's database, both trimmed of surrounding spaces, ignoring case.

A check with no server or no service name serves no such target.

### MATCH-02 — Match a database instance target

A check with a direct connection serves a database instance target when its database identifier, which the check renders for its own telemetry, equals the target's name exactly.

### MATCH-03 — Match from what the check already knows

Matching reads only the check's loaded configuration and the database identifier its scheduled run last produced. It never connects to the database or runs SQL. A check whose configuration is not a direct connection answers not found for every target.

### MATCH-04 — Fail when the database identifier is unknown

A check with a direct connection whose scheduled run has not yet determined its database identifier answers a database instance target with an error, never with not found.

### MATCH-05 — Match again before executing

A query request checks MATCH-01 to MATCH-04 on its check before connecting. When the check no longer serves the target, the run fails as target not found.

## Running the query

### QUERY-01 — Connect as the check, on a connection of the run's own

Each run opens one database connection with its check's connection settings: server, port, service name, user, password, and, with TCPS, the wallet. The connection is used only by that run and is closed when the run ends. A run never takes, or waits for, a connection the check uses for monitoring.

### QUERY-02 — Run only queries

Before connecting, the integration checks the query's first word, after any whitespace, comments, and opening parentheses. The run fails as an unsupported query unless that word is `SELECT`, or `WITH` not followed by a `FUNCTION` or `PROCEDURE` declaration.

Why: Oracle runs DDL inside a read-only transaction, because DDL commits first, and PL/SQL declared in a query can write in an autonomous transaction. The read-only transaction (QUERY-03) cannot stop either.

### QUERY-03 — Run in one read-only transaction

Every statement of the query runs in one transaction that starts with `SET TRANSACTION READ ONLY`. The transaction is rolled back when the run ends, whatever its outcome.

The check's login is the boundary for what a query can reach. A query can still call stored functions; one declared with an autonomous transaction can write wherever the login may.

### QUERY-04 — Fix the session settings

Before the transaction starts, the integration sets, for the run's session only:

- `TIME_ZONE`: `UTC`
- `NLS_NUMERIC_CHARACTERS`: `.,`
- `NLS_DATE_FORMAT`: `YYYY-MM-DD"T"HH24:MI:SS`
- `NLS_TIMESTAMP_FORMAT`: `YYYY-MM-DD"T"HH24:MI:SS.FF`
- `NLS_TIMESTAMP_TZ_FORMAT`: `YYYY-MM-DD"T"HH24:MI:SS.FFTZH:TZM`

The check's monitoring connections keep their own settings.

Why: Conversions that a query makes itself, such as `TO_CHAR` of a date or a date concatenated to text, then give the same text on every database. They do not change how the integration encodes result values (ORA-02).

### QUERY-05 — Bound the database time

The database time ends at the run time limit (TIME-01), or after the instance's remote query timeout when that is set and sooner. When it ends, the driver asks the database to cancel the running call, and the run fails as a timeout. The driver's read timeout on the connection is the time left in the database time, at least 1 s. The instance setting can shorten a run's database time, never lengthen it.

### QUERY-06 — Report database errors by their code

When the database rejects the query, fails while running it, or refuses the connection, the failure message gives the error's Oracle code, such as `ORA-00942`. It never includes the database's or the driver's message text. A database time limit fails the run as a timeout, and a cancellation as cancelled (CANCEL-01).

## Describing the columns

### DESC-01 — Describe the result before reading any row

The integration reads the result's column names and driver types after the query starts and before it reads any row, including for a result with no rows.

### DESC-02 — Name each column's types

For each column, the database type name and the logical type come from its driver type, as in the annex. A `NUMBER` column's type name carries its precision and scale when the column has them, such as `NUMBER(10)` or `NUMBER(5,2)`.

### ORA-01 — Reject columns whose values the run would read whole

Before uploading anything, the integration fails the query when a result column is a `CLOB`, `NCLOB`, `BLOB`, `BFILE`, `LONG`, `LONG RAW`, or `JSON` value, or has a type the annex does not list. The error names the column and suggests selecting a bounded part, with `DBMS_LOB.SUBSTR` or `JSON_SERIALIZE(… RETURNING VARCHAR2)`, or converting it with `TO_CHAR` or `CAST`.

Why: The driver reads a LOB or LONG value whole, up to 1 GiB, before the integration can compare it with the row size limit, so these values would make a run's memory unbounded (RES-01).

### DESC-03 — Reject results a descriptor cannot carry

Before uploading anything, the integration fails the query when its result:

- has no columns;
- has more columns than the column limit;
- has a column name that is empty, longer than 255 bytes, or not valid UTF-8;
- has two columns with the same name, which the error names; or
- needs a schema, and its encoded schema is larger than the schema size limit, or leaves no room for rows in a result file.

### DESC-04 — Register the descriptor before reading rows

The integration registers the descriptor with intake before it reads the first row. The descriptor gives the JSON-cell upload format, the schema choice, the Agent's hostname as the Agent reports it, and the columns in result order with their names, database type names, and logical types.

## Encoding rows

### ORA-02 — Encode each value by its column type

Each cell holds one JSON value, as the annex gives for the column's type. SQL null is `null`. Numbers of `NUMBER` columns are the driver's exact decimal text, never converted through a float. Temporal values are ISO 8601 text. A value that its column's encoding cannot represent, such as text that is not valid UTF-8 or a date outside years 1 to 9999, fails the run as an unsupported value, naming the column.

## Building pages

### SIZE-01 — Size each page by its result file

The integration closes a page before its estimated result file would exceed the result file size target. The estimate includes the file's header and schema, the column names and JSON punctuation that every row repeats in the result file, and, for each cell, the larger of its stored size and the redaction marker that sensitive-data scanning may put in its place.

The estimate can fall short, for example when intake escapes a character that the cell did not. A file that ends up larger than the target is still accepted, up to intake's result file cap (intake PAGE-04).

### SIZE-02 — Split a page that intake reports as oversized

When intake reports that a page's result file exceeds its cap (intake PAGE-05), the integration uploads a smaller page under the same page index, holding the first half of the rejected page's rows, rounded down. The rows it left out start the next page. The query is not run again. When intake reports a page of one row as oversized, the run fails as a row too large.

### SIZE-03 — Keep rows whole and in order

A page holds complete rows, each one CSV record of cells, in result order.

### SIZE-04 — Bound each row

When a row's estimated size in the result file is larger than the row size limit, the run fails as a row too large.

### SIZE-05 — Represent an empty result

A result with no rows uploads one page with no records, with the schema when it is requested (intake OUTPUT-05), and finalizes with a count of one.

### SIZE-06 — Stay within the run's limits

Before uploading a page, the integration fails the run as a result too large when:

- the page would exceed the page count limit;
- the estimated result files of the run's pages so far, with this one, would exceed the total result size limit; or
- the page is larger than intake's [page size](#limits).

## Uploading pages

### UPLOAD-01 — Upload pages one at a time, in order

The integration uploads one run's pages one at a time, in page index order starting at 0. Each page's record offset is the number of rows in all earlier pages. It reads no further row while a page uploads, and sends the next page only after intake acknowledges the previous one.

### UPLOAD-02 — Declare each page

Each page upload declares its page index, exact uncompressed byte length, row count, and record offset, and sends a fixed-length body (intake PAGE-01).

### UPLOAD-03 — Send the same page on every attempt

Every attempt of one page sends the same page index, declared values, and uncompressed bytes. Retries resend the compressed body made for the first attempt.

Why: An attempt whose acknowledgement was lost may have been recorded, so its retry must match it exactly to succeed (intake RETRY-02).

### UPLOAD-04 — Retry requests that can still succeed

The integration retries a descriptor registration, page upload, finalize, or abort as follows:

- **Intake is at capacity (intake PAGE-02):** retried until the run time limit, or the abort time limit for an abort.
- **A transport failure, an attempt that ended without an answer (UPLOAD-05), or an internal error:** retried up to the attempts limit; the run then fails as an upload failure.
- **Any other error:** not retried; the run fails as an upload failure whose message gives intake's error code, except for an oversized page (SIZE-02). A code intake does not document is reported as `invalid_response`.

Waits between attempts grow exponentially, with jitter, from the initial to the maximum [backoff](#limits). No attempt starts after the run time limit.

### UPLOAD-05 — Bound each attempt

An attempt ends without an answer when:

- connecting, including the TLS handshake, takes longer than the connect time limit;
- sending the body makes no progress for the send stall limit; or
- the attempt takes longer than its attempt time limit.

### UPLOAD-06 — Send requests only to the upload address

Every intake request goes to the address the core Agent adds to the delivery instructions (Agent INPUT-05), with the operation's path appended. The integration follows the Agent's proxy configuration, verifies intake's TLS certificate even when the Agent skips verification for other traffic, and never follows a redirect: a redirect answer fails the run as an upload failure.

Why: Every intake request carries the Agent's API key (UPLOAD-07), which a redirect could send to another host.

### UPLOAD-07 — Authenticate with the Agent's API key alone

Every intake request carries the Agent's API key and no other credential (intake AUTH-01).

### UPLOAD-08 — Compress page bodies

The integration compresses each page with gzip at the fastest level, and sends it with `Content-Encoding: gzip`. It sends a page uncompressed when compressing does not make it smaller. When intake answers that it does not accept the encoding, the integration sends that page and the run's later pages uncompressed. This fallback is not counted as a retry attempt (UPLOAD-04).

## Finishing the run

### FINISH-01 — Finalize with the number of pages intake accepted

After the last page, the integration releases the database connection, then finalizes the session with the number of pages intake accepted (intake FINISH-01), retrying as in UPLOAD-04.

### FINISH-02 — Report success only after finalizing

The integration answers success only after intake has confirmed the finalize. The answer carries no upload ID, counts, or totals (its-agent RESULT-01).

### ABORT-01 — Abort the upload of a failed run

When a run that passed request validation fails, the integration aborts its upload session (intake ABORT-01) with the run's failure code and message, the same as its answer to the Agent (OBS-01), including after the run time limit. The abort is bounded by the abort time limit. An abort failure is logged and never changes the run's answer. The integration never aborts a session after its finalize succeeded. A run without an API key (REQ-03) cannot authenticate an abort, and logs that it did not send one.

Why: The worker learns that a run failed, and why, only from its session (its-agent RESULT-01).

## Time and cancellation

### TIME-01 — Bound the run by its time limit

The run time limit starts when the integration receives the query request. It covers matching, connecting, describing, reading, uploading, retries, and finalizing, but not the abort. When the limit passes, the run stops reading and uploading, fails as a timeout, and aborts its session (ABORT-01).

### CANCEL-01 — Stop a cancelled run

When the Agent unschedules a run's check (Agent RUN-03), or the run's database time ends (QUERY-05), the integration:

- asks the database, through the driver, to cancel the run's current call, which also stops a statement that has not yet returned its first row;
- closes the run's database sockets when the driver has not returned within the cancel grace time; and
- stops reading and uploading.

An unscheduled run fails as cancelled and aborts its session (ABORT-01).

### CANCEL-02 — Refuse requests on an unscheduled check

A check that is being unscheduled answers every new match check and query request with a cancelled error, without parsing it. No call starts when the Agent's context for it has already ended.

### CANCEL-03 — Tear the check down only after its runs end

When a check is unscheduled while runs are in progress, its unscheduling returns only after the last run has ended. Cancelling runs never interrupts the check's monitoring queries.

## Errors and observability

### OBS-01 — Answer with one stable code and a safe message

Every failure answer carries exactly one code from the annex and a message that says what failed. Messages may name a result column. They never contain the query text, result values, credentials, connection strings, or the text of a driver error. Unexpected failures, including panics, carry the internal error code and a fixed message.

### OBS-02 — Carry the run's trace to intake

When the query request carries a trace context, every intake request of the run carries it in the Datadog trace headers `x-datadog-trace-id`, `x-datadog-parent-id`, and `x-datadog-sampling-priority`. A request without a trace context carries no trace headers.

### OBS-03 — Log without data

Every log line about a run carries its check, and its crawl, task, and upload IDs. Logs never contain the query text, result values, credentials, or connection strings. Each run's outcome is logged at info level with its code, page and row counts, and duration. Each acknowledged page is logged at debug level with its index, row count, uncompressed and sent byte counts, encoding (`gzip` or `identity`), compression and upload times in milliseconds, and whether gzip was tried but not kept because the page did not get smaller.

## Configuration

### CONFIG-01 — Let an instance shorten remote query time

The instance option `remote_queries.timeout_ms` shortens the database time of remote queries on that instance (QUERY-05). It is unset by default, so the run time limit applies. When set, it is a number of milliseconds, at least 1; a smaller value fails the check's configuration.

## Scalability

### SCALE-01 — Keep monitoring unaffected

While runs are in progress on a check, the check's scheduled runs keep their collection interval and use no connection a run holds.

### SCALE-02 — Keep runs independent

Runs on one check, or on different checks, never wait on one another.

## Resource use

### RES-01 — Hold at most one page per run

A run holds in memory at most one page and its compressed body, the driver's prefetched rows, and the row being encoded. A page is at most the result file size target plus one row. A compressed body larger than its page is never kept.

### RES-02 — Use one database connection per run

A run holds exactly one database connection, from connecting until its rows are read (QUERY-01, FINISH-01).

## Operations

| Operation | Caller | Does |
|---|---|---|
| Resolve | Agent | Says whether this check serves the target (MATCH-01 to MATCH-04) |
| Execute | Agent | Runs the query and uploads its result (MATCH-05 to FINISH-02) |

The integration uses these intake operations:

| Intake operation | Integration uses it to |
|---|---|
| Register descriptor | Register the columns before reading rows (DESC-04) |
| Upload page | Upload each page (UPLOAD-01 to UPLOAD-05, UPLOAD-08) |
| Finalize | Finish the upload (FINISH-01) |
| Abort | Stop the upload of a failed run, with its reason (ABORT-01) |

## Limits

| Limit | Bounds | Value |
|---|---|---|
| Run time limit | One run, from the request to the finalize | The delivery instructions' query time limit |
| Result file size target | The size each result file aims for, best effort (SIZE-01) | The delivery instructions' result file size, at most the page memory limit |
| Page memory | The result file size target | 16 MiB |
| Page size | One page's uncompressed bytes (intake PAGE-01) | 100 MiB |
| Row size, page count, total result size, column, schema size limits | As named | The delivery instructions' limits |
| Database time | Starting the query and reading its rows | The time left in the run, or the instance's remote query timeout when sooner |
| Cancel grace | From the driver's cancel request to closing the run's sockets | 5 s |
| Attempts | Attempts of one request for failures other than capacity | 5 |
| Backoff | Wait between attempts | 100 ms, doubling up to 5 s, with jitter |
| Connect time | Connecting to intake, including TLS, per attempt | 10 s |
| Send stall | Time sending a request body may make no progress | 30 s |
| Page attempt time | One page upload attempt | 6 minutes |
| Other attempt time | One descriptor or finalize attempt | 30 s |
| Abort time | One abort, including its retries | 10 s |
| Abort message | The bytes of the failure message an abort carries | 1,024 |

## Verification

Each item is checked at one layer. Unit tests of the request, page, and intake behavior run in the shared `pkg/collector/remotequeries/session` package; the other unit tests run in the Oracle check package.

| Layer | Items | Dependencies |
|---|---|---|
| Unit | REQ-01, REQ-02, REQ-03, MATCH-01, MATCH-02, MATCH-03, MATCH-04, MATCH-05, QUERY-01, QUERY-02, QUERY-05, QUERY-06, DESC-02, ORA-01, ORA-02, DESC-03, DESC-04, SIZE-01, SIZE-02, SIZE-03, SIZE-04, SIZE-05, SIZE-06, UPLOAD-01, UPLOAD-02, UPLOAD-03, UPLOAD-04, UPLOAD-05, UPLOAD-06, UPLOAD-07, UPLOAD-08, FINISH-01, FINISH-02, ABORT-01, TIME-01, CANCEL-02, CANCEL-03, OBS-01, OBS-02, OBS-03, CONFIG-01 | a mock database driver; a stub intake that decodes gzip pages and can refuse, fail, report a page as oversized, redirect, refuse the encoding, or stall; a short run time limit |
| Check with Oracle and a stub intake (`oracle_test`) | QUERY-03, QUERY-04, DESC-01, CANCEL-01 | the integration-test Oracle database with a SYS login; the go-ora driver |
| Not tested: design | SCALE-01, SCALE-02, RES-01, RES-02 | code review |

## Open questions

1. MATCH-01, MATCH-02: checks that connect through `tns_alias` or an Oracle Client (godror) serve no target. Should they, and how is a TNS alias matched to a host, port, and service target?
2. MATCH-04: a check whose database never answers keeps its database identifier unknown, so every database instance target on its Agent fails to resolve. Should it answer not found instead after some time?
3. QUERY-02, QUERY-03: Oracle has no read-only guarantee for stored functions with autonomous transactions. Is the check's login, with the [its-agent](https://dd-quick.us1.staging.dog/its-agent-spec/) trusted-query gate, enough, or should the integration require a login without write privileges?
4. ORA-01: should LOB values be read with a bound, through the driver's LOB locators, instead of being rejected?
5. QUERY-01: should a check bound its concurrent runs, to protect the customer database (Agent RUN-04)?

## Annex: column types

The database type name, logical type (DESC-02), and cell (ORA-02) of each column, by its Oracle type:

| Oracle type | Database type name | Logical type | Cell |
|---|---|---|---|
| `NUMBER`, `FLOAT`, `INTEGER`, `BOOLEAN` as the driver reports it | `NUMBER`, with precision and scale when the column has them | decimal | The exact decimal text, written as a JSON number; intake stores it as a string (intake INPUT-07) |
| `BINARY_FLOAT`, `BINARY_DOUBLE` | `BINARY_FLOAT`, `BINARY_DOUBLE` | float | The shortest JSON number that reads back as the same value; `"NaN"`, `"Infinity"`, or `"-Infinity"` for those values |
| `VARCHAR2`, `NVARCHAR2` | `VARCHAR2` | string | The text |
| `CHAR`, `NCHAR` | `CHAR` | string | The text, with its blank padding |
| `ROWID`, `UROWID` | `ROWID`, `UROWID` | string | The text |
| `DATE` | `DATE` | temporal | `YYYY-MM-DDTHH:MM:SS` |
| `TIMESTAMP` | `TIMESTAMP` | temporal | `YYYY-MM-DDTHH:MM:SS`, with the fraction of a second without trailing zeros |
| `TIMESTAMP WITH TIME ZONE` | `TIMESTAMP WITH TIME ZONE` | temporal | As `TIMESTAMP`, with the value's offset, such as `+05:30` |
| `TIMESTAMP WITH LOCAL TIME ZONE` | `TIMESTAMP WITH LOCAL TIME ZONE` | temporal | As `TIMESTAMP`, in UTC, ending in `Z` |
| `INTERVAL YEAR TO MONTH`, `INTERVAL DAY TO SECOND` | as named | temporal | The driver's text, such as `+01-02` or `+03 04:05:06.700000` |
| `RAW` | `RAW` | binary | `\x` followed by the bytes in lowercase hex |
| `CLOB`, `NCLOB`, `BLOB`, `BFILE`, `LONG`, `LONG RAW`, `JSON`, and any other type, including `XMLType`, object, collection, and `VECTOR` types | — | — | Rejected (ORA-01) |

## Annex: failure codes

| Code | Failure |
|---|---|
| `invalid_request` | The request failed REQ-01 or REQ-02 |
| `target_not_found` | The check does not serve the target, or no longer does (MATCH-01 to MATCH-03, MATCH-05) |
| `target_unavailable` | The check's database identifier is not known yet (MATCH-04) |
| `credentials_unavailable` | The Agent has no API key (REQ-03) |
| `unsupported_query` | The query is not a `SELECT` or `WITH` query (QUERY-02) |
| `connection_failed` | The database refused or dropped the connection (QUERY-06) |
| `query_failed` | The database rejected the query or failed while running it (QUERY-06) |
| `unsupported_column` | A column has a type that ORA-01 rejects |
| `invalid_columns` | The result has no columns, too many, unusable names, or duplicate names (DESC-03) |
| `schema_too_large` | The schema is larger than the schema size limit, or leaves no room in a result file (DESC-03) |
| `unsupported_value` | A value its column's encoding cannot represent (ORA-02) |
| `row_too_large` | A row exceeds the row size limit, or a one-row page is oversized (SIZE-02, SIZE-04) |
| `result_too_large` | The result exceeds the page count, total result size, or page size limit (SIZE-06) |
| `upload_failed` | Intake rejected a request, or retries ran out (UPLOAD-04, UPLOAD-06) |
| `timeout` | The run time limit or the database time passed (QUERY-05, TIME-01) |
| `cancelled` | The Agent unscheduled the check (CANCEL-01, CANCEL-02) |
| `internal_error` | Any other failure (OBS-01) |
