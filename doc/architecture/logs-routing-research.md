# Agent log routing: research notes

## Scope and evidence

These notes explain existing log delivery and its relationship to selective multi-org routing. They do not propose an implementation or development plan.

The target outcome is selective log delivery to Datadog orgs, based on JSON content or tags. Configuration design and the future filter DSL remain outside this research.

The local baseline is revision `9d8ea371d02`. The pre-intake comparison uses open [PR #58698][pr] at commit `046cf48e95788600262fc788f8a4054413381925`.

The PR snapshot differs from the local baseline. The comparison describes proposed behavior, not proof of released Agent behavior.

No tests ran. The notes distinguish source observations, architectural conclusions, and open questions.

## 1. The most important distinction

**Selective multi-org routing answers: which org destinations should receive this log?**

**Pre-intake routing answers: which intake host should this existing org destination use?**

Pre-intake routing does not replace a JSON or tag selector. It does not create selective batches for multiple orgs.

Conceptually, both decisions can apply to one delivery path:

```text
Log content and Agent metadata
  → Select org destination(s)       target outcome
  → Make destination-compatible batches
  → Resolve each org's intake host  pre-intake proposal
  → Deliver HTTP payloads
```

This diagram separates responsibilities. It does not prescribe an implementation or exact runtime call order.

Evidence: [Slack discussion][slack] and [proposed endpoint copying][pr-endpoints].

## 2. Current log delivery

For the normal, non-serverless HTTP path:

```text
Source → Tailer / decoder → Processor → Batch strategy
                                           ↓
                                     Shared sender
                                           ↓
                                  HTTP destinations
                                           ↓
                                     Datadog intake

Destination completion → Auditor → Source checkpoints
```

A pipeline is a local processing lane, not an org. Several pipelines can feed one shared sender. Each endpoint becomes a destination with separate HTTP delivery state.

The Agent connects the auditor as the sender output sink.

The logs README gives useful background. Some paths and ordering statements do not match this checkout, so the implementation is the authority.

Read [Agent setup][setup], [provider construction][provider], and [pipeline construction][pipeline].

## 3. Message, metadata, and payload

A `Message` contains one log and its metadata. A `Payload` contains encoded batch bytes and message metadata. The sender distributes payloads rather than individual application JSON objects.

`Origin` supplies Agent service, source, and tags. The service setting takes precedence over the parser-derived service.

The Agent service field differs from an arbitrary `service` property inside application JSON. MRF reads the former, not the latter.

An outgoing JSON envelope does not prove the Agent parsed application JSON into searchable attributes. The envelope and application content are different forms of structure.

The code supports structured messages for certain inputs. That support is not a general JSON filter DSL.

Read [message types and states][message], [service resolution][origin], and the [JSON encoder][json].

## 4. Multiple endpoints and reliability

An `Endpoint` contains host, port, credentials, TLS, intake path, compression, retry settings, reliability, and MRF identity. `Endpoints` groups the main endpoint and extra endpoints.

The HTTP destination sends its API key in the `DD-API-KEY` header. Different org credentials can use the same intake host. A host alone is not an org identity.

**Normal extra endpoints receive copies of the same encoded payload.** The current general sender does not apply different JSON or tag filters for each endpoint.

The main endpoint is reliable. Extra endpoints default to reliable unless their settings say otherwise.

For ordinary active destinations:

- The worker tries reliable destinations first.
- A destination in retry state does not stop another healthy reliable destination.
- The worker tries to buffer copies for skipped reliable destinations. Those copies can drop if their buffers fill.
- Unreliable destinations use nonblocking sends and do not advance the auditor.
- Unreliable destinations do not independently sustain delivery when all ordinary reliable destinations remain stuck.

**Reliable does not mean every org must receive every log.** MRF has special skip paths, and permanent errors can cause drops.

Read [endpoint definitions][endpoints], the [HTTP factory][http-factory], [worker distribution][worker], and [DestinationSender][destination-sender].

## 5. Processor and batching

The processor applies global and source processing rules. It then renders the message, marks MRF eligibility, and encodes the outgoing record.

The HTTP encoder creates a JSON envelope with `message`, `service`, `ddsource`, `ddtags`, `hostname`, `status`, and `timestamp` fields.

The batch strategy accepts encoded messages. It keeps separate `main` and `mrf` batches.

Each batch serializes records into a JSON array and compresses the array as it grows. It retains message metadata for accounting and checkpoints.

A batch flushes after a count limit, byte limit, timer event, explicit flush, or input closure.

The byte limit counts encoded record bytes before compression. It does not count compressed network bytes. A message that cannot fit in an empty batch drops here.

**Compression happens before endpoint distribution.** Destinations reuse the payload bytes rather than independently encoding each log.

Read [message processing][processor], [batch selection][batch-strategy], [batch construction][batch], and [buffer limits][buffer].

## 6. HTTP transport

Each HTTP destination builds a URL and POST request. It adds authentication, content type, content encoding, and timestamp headers.

Each destination owns an HTTP client, request worker pool, retry state, and backoff policy.

For reliable destinations, network failures and HTTP 429 or server errors normally cause retries. HTTP 400, 401, 403, and 413 normally cause drops.

The 403 path has a secret-refresh exception. If refresh succeeds, the destination retries.

The worker-pool code supports dynamic concurrency. The normal provider currently sets equal minimum and maximum concurrency, so this path uses fixed concurrency.

A successful `DestinationSender.Send` call can mean queue acceptance. It does not prove successful intake delivery.

Read [HTTP retry handling][http-retry], [HTTP requests][http-request], and [provider concurrency][provider-concurrency].

## 7. Multi-Region Failover

### Setup and activation

The `multi_region_failover.enabled` setting creates MRF infrastructure. Endpoint construction adds a secondary endpoint with its own API key and `IsMRF` flag.

The `multi_region_failover.failover_logs` setting activates log forwarding. **HTTP errors do not automatically activate MRF in the inspected logs path.**

The remote-config client subscribes to `AGENT_FAILOVER` updates and applies runtime settings. HTTP errors separately control retries and backoff.

Read [MRF endpoint construction][mrf-endpoint], the [remote-config callback][mrf-rc], and the [destination activation check][destination-sender].

### Message selection

While log failover is active, the processor checks `multi_region_failover.logs_service_allowlist`:

- An empty list selects all services.
- A nonempty list selects exact matches against `msg.Origin.Service()`.

Selected messages receive `IsMRFAllow`. This is internal delivery metadata, not an ordinary Datadog tag string.

### Separate batches and destination checks

The batcher separates eligible and ineligible messages. `Payload.IsMRF()` checks the first message because every message in a payload has the same eligibility.

The sender skips non-MRF payloads for MRF destinations. **The primary destination still receives both eligibility classes.**

With failover active and a `payments` allowlist:

```text
payments logs → primary + MRF destination
other logs    → primary destination
```

MRF provides narrow service-based selective forwarding. It is not a general JSON or tag routing engine, and it does not remove selected logs from primary delivery.

Trace [message selection][mrf-filter], [batch selection][batch-strategy], [payload eligibility][payload-mrf], and [destination checks][worker].

## 8. Auditor, checkpoints, and failover names

Reliable destinations send completed payloads to the auditor channel. Unreliable destinations use a drain channel.

The auditor updates source offsets from payload metadata and saves a registry. It does not keep a separate checkpoint for each org.

**Completed does not always mean successful intake delivery.** HTTP destinations also forward payloads after permanent errors, so drops can advance checkpoints.

Read [destination completion][http-retry] alongside [auditor updates][auditor].

Three mechanisms share similar names:

| Mechanism | Responsibility |
|---|---|
| Multi-Region Failover | Secondary destination activation and message eligibility |
| Pipeline failover | Local processing-lane selection when another lane blocks |
| HTTP/TCP fallback | Transport selection after connectivity checks |

Pipeline failover does not select an org. Concurrent HTTP sends and optional pipeline failover also prevent the pipeline diagram from proving strict intake ordering.

Read [pipeline failover][pipeline-failover] and [transport setup][transport].

## 9. What the Slack discussion describes

### Pre-intake discovery

The proposal discovers a backend-selected intake host for an existing destination. It does not inspect individual log JSON or tags.

Routes group by site and API key. The component sends one query for each group. Two org keys on the same site get separate answers.

The provisional query uses the group API key. Its response maps telemetry types to optional host overrides.

```http
GET https://api.<site>/api/unstable/agent/intake-routing
DD-API-KEY: <org key>
```

```json
{"routes":{"logs":{"override":true,"host":"<host>","port":443}}}
```

**The backend API is not final.** The architecture document labels this wire format provisional.

Read [routes and groups][pr-groups] and the [provisional protocol][pr-protocol].

### Host changes, not org changes

The logs integration copies configured endpoints and changes host and port. The copies retain their credential references.

It does not replace the org API key or decide which messages belong to another org. This is the strongest implementation evidence for the responsibility distinction.

Read [`WithHostOverrides`][pr-endpoints] and [logs registration][pr-integration].

### Startup and updates

The logs agent registers eligible endpoints and waits for an initial decision within a short grace period. The documented default grace period is one second.

The component refreshes backend answers in the background. The documented default refresh interval is six hours.

A host change signals the logs agent. The logs agent compares endpoints and uses its existing partial restart path when hosts differ.

The architecture document says this restart retains tailers, auditor, sources, and schedulers. It also warns about short pipeline pauses and duplicate log delivery.

Read [startup][pr-startup], [logs integration][pr-logs], and the [restart callback][pr-restart].

### Eligibility and safeguards

The inspected PR supports eligible main and extra HTTP endpoints. It excludes MRF endpoints, Observability Pipelines Worker endpoints, and custom hosts.

Candidates use the official logs intake host, HTTPS, the v2 API, no path prefix, and port zero or 443.

The component also checks proxy use and DNS results. It preserves the default path when a proxy or private address prevents direct public delivery.

An override must pass hostname validation and an authenticated HTTPS probe. The PR allows overrides under official Datadog domains, including another site.

Failed queries or failed probes generally keep the current endpoint. An explicit no-override answer restores the default endpoint.

The architecture document also notes a transport constraint. Extra endpoints use TCP unless `logs_config.force_use_http` is true. Without that setting, this HTTP-only feature does not apply.

Read [candidate selection][pr-candidates], [safety checks][pr-safety], and [extra endpoints and TCP][pr-tcp].

## 10. Does it conflict with the objective?

### Conclusion

**No direct conflict appears between the stated goals. They solve different decisions.** The current PR does not satisfy selective multi-org routing by itself.

A conceptual example makes the difference clear:

```text
Selective routing: payments log belongs to org A
Pre-intake lookup: org A currently uses dedicated intake host A
Delivery: send the selected log with org A credentials to host A
```

This compatibility is an architectural conclusion, not a tested integration guarantee.

### Shared implementation concerns

- Both efforts affect endpoint handling and logs-agent lifecycle code.
- Future org destinations need the correct credential identity during intake-host discovery.
- Destination selection and intake-host resolution must not become the same identity concept.
- Pre-intake host changes currently cause pipeline restarts, so batching and checkpoint behavior matter.
- MRF has its own destination rules and is outside the current pre-intake eligibility set.
- Custom, proxy, and private-network destinations can keep their configured address.

These are coordination questions, not an implementation plan or confirmed defects.

### Questions for the authors

1. What interface should future logical org destinations use for intake-host discovery?
2. Should selective org destinations participate without depending on the current extra-endpoint configuration surface?
3. What state must survive a pre-intake host change during batching or delivery?
4. How should pre-intake resolution and MRF compose?
5. Where does Foldspace implement the same delivery responsibilities?

## 11. Reading order and limits

Read the message types first. Then follow one message through processing, batching, sender distribution, HTTP delivery, and auditor progress.

Repeat that trace with `IsMRFAllow` set. Compare the batch split and destination checks.

Next, read the pre-intake host-copy helper and change callback. They show how proposed endpoint resolution differs from message selection.

Useful tests, inspected but not executed:

- [Service selection test](../../comp/logs-library/processor/processor_test.go#L379).
- [Separate MRF batches test](../../comp/logs-library/sender/batch_strategy_test.go#L292).
- [MRF activation test](../../comp/logs-library/sender/destination_sender_test.go#L166).
- [One reliable destination fails test](../../comp/logs-library/sender/worker_test.go#L241).

No tests ran. Foldspace had no matches in the tracked-content and filename searches. Its external architecture remains unverified.

The backend activation policy, final pre-intake API, and production behavior remain unverified. The separate high-level Confluence workstream document was not inspected.

## Sources

Local baseline: `9d8ea371d02`. Local links refer to that research snapshot, not a permanent line-number contract.

Meeting notes: `/Users/ella.taira/Documents/Obsidian Vault/ella.taira/4q206/routing.md`.

Discussion: [Slack thread][slack].

Proposed changes: [PR #58698][pr] and [architecture document][pr-architecture].

[setup]: ../../comp/logs/agent/impl/agent_core_init.go#L114

[provider]: ../../comp/logs-library/pipeline/provider.go#L85

[pipeline]: ../../comp/logs-library/pipeline/pipeline.go#L34

[message]: ../../pkg/logs/message/message.go#L33

[origin]: ../../pkg/logs/message/origin.go#L184

[endpoints]: ../../comp/logs/agent/config/endpoints.go#L56

[http-factory]: ../../comp/logs-library/sender/http/http_sender.go#L86

[worker]: ../../comp/logs-library/sender/worker.go#L95

[destination-sender]: ../../comp/logs-library/sender/destination_sender.go#L78

[processor]: ../../comp/logs-library/processor/processor.go#L190

[json]: ../../comp/logs-library/processor/json.go#L36

[batch-strategy]: ../../comp/logs-library/sender/batch_strategy.go#L102

[batch]: ../../comp/logs-library/sender/batch.go#L93

[buffer]: ../../comp/logs-library/sender/message_buffer.go#L29

[http-retry]: ../../comp/logs-library/client/http/destination.go#L270

[http-request]: ../../comp/logs-library/client/http/destination.go#L366

[provider-concurrency]: ../../comp/logs-library/pipeline/provider.go#L163

[mrf-endpoint]: ../../comp/logs/agent/config/config.go#L514

[mrf-rc]: ../../comp/remote-config/rcclient/impl/rcclient.go#L180

[mrf-filter]: ../../comp/logs-library/processor/processor.go#L238

[payload-mrf]: ../../pkg/logs/message/message.go#L539

[auditor]: ../../comp/logs/auditor/impl/auditor.go#L270

[pipeline-failover]: ../../comp/logs-library/pipeline/provider.go#L375

[transport]: ../../comp/logs/agent/impl/agent_core_init.go#L60

[slack]: https://dd.slack.com/archives/C089UJ6CUAW/p1792693286117339

[pr]: https://github.com/DataDog/datadog-agent/pull/58698

[pr-architecture]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/intakerouting/ARCHITECTURE.md

[pr-groups]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/intakerouting/ARCHITECTURE.md#routes-and-groups

[pr-protocol]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/intakerouting/ARCHITECTURE.md#wire-protocol-provisional

[pr-startup]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/intakerouting/ARCHITECTURE.md#startup

[pr-logs]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/intakerouting/ARCHITECTURE.md#how-the-logs-agent-uses-the-component

[pr-safety]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/intakerouting/ARCHITECTURE.md#safety-checks

[pr-tcp]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/intakerouting/ARCHITECTURE.md#additional-endpoints-and-tcp

[pr-endpoints]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/logs/agent/config/intake_routing.go#L54-L82

[pr-candidates]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/logs/agent/config/intake_routing.go#L22-L51

[pr-integration]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/logs/agent/impl/agent_intakerouting.go#L53-L88

[pr-restart]: https://github.com/DataDog/datadog-agent/blob/046cf48e95788600262fc788f8a4054413381925/comp/logs/agent/impl/agent_intakerouting.go#L91-L128
