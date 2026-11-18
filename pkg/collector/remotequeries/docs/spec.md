# Datadog Agent remote queries: specification

## Purpose

The Datadog Agent runs Remote Queries next to customer databases. Its private action runner receives the Remote Queries action from Action Platform (AP), sent by the `its-agent` worker, and passes it to the core Agent. For a match check, the core Agent reports whether exactly one of its loaded integration checks serves the requested database. For a query request, it finds that check again, hands it the query and delivery instructions, and answers as soon as the check has the run. The integration then runs the query and uploads the results to `its-agent-intake`.

The Agent does not validate SQL, connect to databases, convert or upload results, choose among Agents, or report run status to ITS. Result rows never pass through the core Agent or the runner. Integration behavior, such as matching a check to a target and running the query, is specified per integration, starting with the [Postgres integration](https://dd-quick.us1.staging.dog/postgres-integration-spec/).

## Terms

- **Runner:** the private action runner, the Agent process that receives AP actions.
- **Core Agent:** the Agent process that loads and schedules integration checks.
- **Action:** the AP action `com.datadoghq.remoteaction.integrations.runQuery`, the only Remote Queries action ([its-agent](https://dd-quick.us1.staging.dog/its-agent-spec/) AP-02).
- **Integration:** the Agent integration the action names, such as `postgres`.
- **Check:** one loaded instance of an integration, built from one instance configuration.
- **Target:** the database the action names: either a host, port, and database name, or a database instance name.
- **Match check:** an action that asks whether exactly one loaded check serves the target, and runs no SQL (its-agent RESOLVE-02).
- **Query request:** an action that runs a query on the target and uploads its results (its-agent EXEC-02).
- **Delivery instructions:** the query request's crawl ID, task ID, upload ID, query time limit, result file size target, and limits (its-agent EXEC-02).
- **Run:** one query request on one check, from the moment the core Agent hands it to the check until the check answers.
- **Site:** the Datadog site the Agent is configured for, such as `datadoghq.com`.

## Enabling

### ENABLE-01 — Run remote queries only where enabled

An Agent runs remote queries only when its remote queries setting is on and the runner's action allowlist, `private_action_runner.actions_allowlist`, names the action. The remote queries setting is off by default. When it is off, the core Agent answers every match check with a resolution error and every query request with a disabled error, and calls no check.

### ENABLE-02 — Accept requests only over the local IPC channel

The runner sends requests to the core Agent only over the Agent's authenticated local IPC channel. The core Agent accepts Remote Queries requests only from callers that authenticate on that channel. No Remote Queries request is accepted from the network.

### ENABLE-03 — Carry no credential in a request

No request carries database credentials, API keys, or upload credentials. The integration connects to the database with its check's configured credentials, and authenticates uploads with the Agent's own API key.

## Checking the request

### INPUT-01 — Select the kind of request with the resolve-only flag

The action input's resolve-only flag selects the kind of request. A match check carries only the integration, the target, and the flag. A match check that carries a query, a schema choice, or delivery instructions, even with an empty value, is invalid. The annex shows both inputs.

### INPUT-02 — Check the integration name

After trimming surrounding spaces and lowercasing, the integration name is non-empty and contains only letters, digits, and `_`.

### INPUT-03 — Parse the target

The target has exactly one of two forms:

- a host, port, and database name, where the host and database name are non-empty strings and the port is an integer from 1 to 65535; or
- a database instance name, a non-empty string without surrounding spaces, with no host, port, or database name key, even an empty one.

A target with any other key, a mix of the two forms, or a missing part is invalid. The Agent compares hosts after trimming surrounding spaces, lowercasing, and removing one trailing dot.

### INPUT-04 — Require the query and complete delivery instructions

A query request has a non-empty query and delivery instructions with:

- a crawl ID and a task ID that are UUIDs (the crawl ID is required on this action path only: the embedded RC tunnel path, which is inactive unless a trusted authority is wired in, has no crawl and omits the field from the delivery instructions);
- an upload ID of letters, digits, `.`, `_`, or `-`;
- a result file size target and limits that are positive integers.

Delivery instructions or limits with an unknown field are invalid. The Agent sets no bound of its own on the result file size target or the limits.

Why: The worker and intake own those values, so an Agent bound could only disagree with them.

### INPUT-05 — Choose the upload address from the Agent's site

The core Agent derives the intake upload address from its locally configured Datadog site. It uses the site's public API host over HTTPS and the intake path shown in the annex. The remote action cannot supply or override the address. An Agent with a missing or malformed site fails a query request before calling a check.

Why: Every upload carries the Agent's API key, so the upload destination belongs to the Agent's local configuration.

### INPUT-06 — Reject an invalid request before calling any check

A request that fails INPUT-01 to INPUT-05 calls no check. An invalid match check is answered with a resolution error, and an invalid query request with an invalid-request error. The message names the field that failed.

## Choosing the check

### CHECK-01 — Consider the loaded checks of the integration

The candidates for a request are the checks the core Agent has loaded whose integration name equals the request's, ignoring case. When there are none, the target is not found.

### CHECK-02 — Ask each check whether it serves the target

The core Agent asks every candidate whether it serves the target, passing it only the target (integration's resolve operation, annex). Each check answers with exactly one of: matched, not found, or an error. The core Agent never reads instance configurations itself to decide a match.

### CHECK-03 — Fail closed on a check that cannot answer

When any candidate cannot give an answer, the resolution is a resolution error, even when another candidate matched. A candidate cannot give an answer when it:

- does not support remote queries;
- answers with an error other than not found;
- gives anything other than exactly one well-formed answer; or
- fails while answering.

A check that is being unscheduled answers with an error ([Postgres integration](https://dd-quick.us1.staging.dog/postgres-integration-spec/) CANCEL-02), so it also gives a resolution error.

Why: A candidate that cannot answer may also serve the target, so another candidate's match could hide an ambiguous target.

### CHECK-04 — Combine the answers

When every candidate answered, the resolution is:

- **matched** when exactly one candidate matched;
- **ambiguous target** when more than one matched;
- **target not found** when none matched.

### CHECK-05 — Resolve again for every query request

A query request resolves its target again, as in CHECK-01 to CHECK-04, and runs only on the one matched check. A match check's answer carries nothing that a query request reuses or must present. When this resolution is not matched, the query request fails with the resolution's code (OUT-03) and no check runs a query.

Why: The loaded checks can change between the match check and the query request.

### CHECK-06 — Keep match checks free of side effects

A match check runs no SQL, contacts no intake, and changes no state on the Agent or the check.

## Running the query

### RUN-01 — Hand the query to the matched check

The core Agent adds its locally resolved upload address (INPUT-05) to the delivery instructions. It passes the matched check the target, the query, the schema choice, these instructions, and the trace context (OBS-01), in one execute call (annex). The check runs the query with its own configuration and credentials. It starts the execute call and answers the query request without waiting for the call to return (OUT-02).

### RUN-02 — Run alongside scheduled check runs and other runs

A run does not wait for its check's schedule, and the check's scheduled runs do not wait for it. Several runs can be in progress at once, on one check or on different checks. The core Agent does not limit how many are in progress.

### RUN-03 — Never start a run on an unscheduled check

When the core Agent unschedules a check, including at shutdown:

- it signals the check before tearing it down;
- it starts no resolve or execute call on the check once it has signaled it; and
- the integration stops the check's runs in progress and delays the check's teardown until they end ([Postgres integration](https://dd-quick.us1.staging.dog/postgres-integration-spec/) CANCEL-01, CANCEL-03).

### RUN-04 — Keep a run going after the query request is answered

A run continues after the core Agent answers its query request, and after the runner's call ends for any reason. Only these stop it:

- its run time limit (Postgres TIME-01);
- the worker aborting its upload session, which makes intake reject its next page (Postgres UPLOAD-04); and
- its check being unscheduled, including at shutdown (RUN-03).

Why: A query can run until the job deadline, which is longer than the runner's task timeout, 60 s by default. The worker learns how the run ended from its upload session (its-agent RESULT-01), not from the runner's call.

## Answering

### OUT-01 — Answer a match check with its resolution

A match check is answered with a status of matched, target not found, ambiguous target, or resolution error. Every status other than matched also carries an error with the same code and a message. A resolution error's message says which candidate could not answer and why, by the check's identifier, never by its configuration.

### OUT-02 — Answer success once the check has the run

When the core Agent has started the matched check's execute call, the action's output has a succeeded status and nothing else.

Why: The worker decides the run from its upload session (its-agent RESULT-01), which the integration finalizes or aborts.

### OUT-03 — Answer a failure before the run with a code and a message

When the query request fails before the core Agent starts the execute call, the action's output has a failed status, and an error with the failure code and a message. Target not found, ambiguous target, and resolution error come from the core Agent's resolution (CHECK-05). The annex lists the core Agent's codes.

### OUT-04 — Log a malformed final answer as a failure

The core Agent logs a run as failed, with a message that names the problem, when the integration:

- ends a run without exactly one final answer; or
- reports both success and failure.

### OUT-05 — Return only the outcome

Only the outcome crosses the core Agent and the runner: a status or an error. Result rows, page contents, and schemas never do. A run whose integration tries to pass result bytes through the core Agent fails.

### OUT-06 — Answer with the documented fields only

The action's output contains exactly the fields shown in the annex, which AP's output schema for the action allows.

## Observability

### OBS-01 — Pass the runner's trace to the integration

The runner passes the trace ID and span ID of its active action span, and the trace's sampling priority, to the integration with the query request. It passes them only when both IDs are non-zero and the priority is 1 or 2. A missing or invalid trace context never affects a run.

### OBS-02 — Log without data

Every Agent log line about a request carries its integration, and its run, task, and upload IDs when it has them. The crawl ID is logged only when the request has one; a run started over the RC tunnel has none, and its log line omits the field rather than logging an empty or substituted one. Logs never contain the query text, result values, credentials, or check configurations.

### OBS-03 — Summarize each request

At the end of each request the core Agent logs one line with its kind, its outcome and code, its duration, the number of candidates, and the matched check's identifier. When a run's execute call returns, the core Agent logs one line with the run's outcome, code, and duration. The call's answer goes nowhere else.

## Performance

Targets are measured on an Agent with no other load, on an idle EC2 `m6gd.4xlarge`, with loaded checks that answer at once, from the runner receiving the action. Each target bounds the median of five runs.

### PERF-01 — Resolve within 1 s

With up to 50 loaded checks of the integration, a match check is answered within 1 s.

### PERF-02 — Start the query within 1 s

With up to 50 loaded checks of the integration, the matched check receives the execute call within 1 s.

## Scalability

### SCALE-01 — Keep requests independent

A match check or query request never waits for another run to finish. A slow or stuck run does not delay another request's resolution or answer.

## Resource use

### RES-01 — Keep Agent memory independent of result size

Outside the integration, the memory the core Agent and the runner use for a run does not depend on the run's number of rows, bytes, or pages.

## Operations

| Operation | Caller | Does |
|---|---|---|
| Action, match check | AP, for the worker | Reports whether exactly one loaded check serves the target (CHECK-01 to CHECK-06, OUT-01) |
| Action, query request | AP, for the worker | Starts the query on the matched check and answers once the check has the run (CHECK-05, RUN-01 to RUN-04, OUT-02 to OUT-06) |
| Core Agent: resolve | Runner, over local IPC | The match check inside the Agent (ENABLE-02) |
| Core Agent: execute | Runner, over local IPC | The query request inside the Agent (ENABLE-02) |
| Integration: resolve | Core Agent | Asks one check whether it serves the target (CHECK-02) |
| Integration: execute | Core Agent | Runs the query on one check (RUN-01) |

## Limits

| Limit | Bounds | Value |
|---|---|---|
| Trace sampling priorities | Priorities passed to the integration | 1 or 2 |

## Verification

Each item is checked at one layer. Performance and resource-use items are checked both by opt-in tests on the machine the Performance section names and on a deployed Agent.

| Layer | Items | Dependencies |
|---|---|---|
| Unit | ENABLE-03, INPUT-01, INPUT-02, INPUT-03, INPUT-04, INPUT-05, CHECK-04, OUT-06 | none |
| Core Agent over local IPC | ENABLE-01, ENABLE-02, INPUT-06, CHECK-01, CHECK-02, CHECK-03, CHECK-05, CHECK-06, RUN-01, RUN-02, RUN-03, RUN-04, OUT-01, OUT-02, OUT-03, OUT-04, OUT-05, OBS-02, OBS-03, SCALE-01 | stub integration checks that can match, fail, stall, or answer wrongly; an IPC client in place of the runner |
| Runner | OBS-01 | a stub core Agent; a tracer that records spans |
| Quality, opt-in | PERF-01, PERF-02, RES-01 | EC2 `m6gd.4xlarge`; stub integration checks |
| Deployed Agent | PERF-01, PERF-02, RES-01 confirmed end to end | staging Agent with a Postgres database, reached through staging AP; Agent memory metrics |

## Open questions

1. ENABLE-01: one remote queries setting, or separate settings for resolve and execute? Does a change take effect without a restart?
2. RUN-02: the runner's task concurrency does not limit runs (RUN-04). Should the Agent limit concurrent runs per check or per Agent, to protect the customer database, or leave that to each integration?
3. INPUT-04: an older Agent rejects delivery instructions with a field a newer worker adds. How does the worker learn which Agent and integration versions can run a request?
4. CHECK-02: does the ClickHouse integration answer its own resolve operation, as CHECK-02 requires of every integration?

## Annex: action input and output

The action input and output are shown in the [its-agent annex](https://dd-quick.us1.staging.dog/its-agent-spec/#annex-action-platform-action). The core Agent's failure codes (OUT-03):

| Code | Failure |
|---|---|
| `target_not_found` | No loaded check of the integration serves the target |
| `ambiguous_target` | More than one loaded check serves the target |
| `resolution_error` | A candidate check could not answer, or a match check was invalid or arrived while remote queries were off |
| `invalid_request` | The query request failed INPUT-01 to INPUT-05 |
| `disabled` | Remote queries are off on this Agent |
| `cancelled` | The query request ended before the core Agent started the run |
| `integration_unavailable` | The matched check does not support remote queries, or the call into it failed |

## Annex: calls into the integration

The core Agent calls a check with one JSON request and receives exactly one final answer (CHECK-02, RUN-01).

A resolve call carries only the target:

```json
{
  "operation": "resolve",
  "target": { "host": "shop-prod-db-01.internal", "port": 5432, "dbname": "orders" }
}
```

Its answer is matched, or an error:

```json
{ "status": "MATCHED" }
```

```json
{ "status": "FAILED", "error": { "code": "target_not_found", "message": "no Postgres instance matches the target" } }
```

An execute call carries the target, the query, the schema choice, the delivery instructions, and the trace context when there is one:

```json
{
  "operation": "execute",
  "target": { "database_instance": "shop-prod-db-01" },
  "query": "SELECT id, email, created_at FROM customers",
  "includeSchema": true,
  "resultDelivery": {
    "crawlId": "7c4e2b1a-9f3d-4c8e-b6a5-1d2f3e4a5b6c",
    "taskId": "d25e7f9f-6883-4462-b31e-b449f70c0894",
    "uploadId": "6f1c2a9e-3b7d-4f0a-9c84-2d5e1b7a0c3f",
    "baseUrl": "https://api.datad0g.com/api/unstable/its-agent-intake",
    "limits": {
      "maxFileBytes": 16777216,
      "maxResultBytes": 107374182400,
      "maxRowBytes": 16777216,
      "maxColumns": 1024,
      "maxSchemaBytes": 1048576,
      "maxPages": 1280,
      "timeoutMs": 3000000
    }
  },
  "traceContext": { "traceId": "5843016738114925931", "parentId": "1122334455667788", "samplingPriority": 1 }
}
```

Its answer is success or an error:

```json
{ "status": "SUCCEEDED" }
```

```json
{ "status": "FAILED", "error": { "code": "timeout", "message": "the query ran past its time limit" } }
```
