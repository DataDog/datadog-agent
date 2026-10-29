# Expected behavior of injected log routing

This PR adds an injected routing mechanism; it does not activate routing or add a production routing policy. A caller defines groups and selects them for each record. Each group owns one sender and one endpoint set. An endpoint is a member of a set, not the owner of another set.

| Selection | Expected delivery and checkpoint behavior |
| --- | --- |
| A only | Only A receives the record. A's terminal sender outcome completes its obligation. A retryable outage retains the record and eventually pauses its source; it never redirects the record to B. |
| B only | The same behavior applies independently to B. Another source stalled on A does not stop this source while aggregate capacity remains. |
| A and B | Both groups receive a copy. B can complete while A is unavailable, but the record remains pending for A. Queue acceptance is not completion. |

There is **one persisted checkpoint per source**. It advances only through the source's completed record prefix. Thus even disjoint A-only and B-only records from the *same source* can affect each other's checkpoint: an earlier pending A record holds a later completed B record behind it. Separate sources have separate checkpoints. A source pauses at its pending limit; exhaustion of the aggregate limit can pause other sources too. After an abrupt restart, a completed B copy may be sent again if A held the checkpoint. Replay requires retained input and an older saved checkpoint; the existing no-checkpoint limitation remains.

## Reliable endpoint sets

Each routed group requires a reliable HTTP primary. It may also contain additional reliable or unreliable endpoints. The existing sender considers a group's reliable set satisfied by **any one reliable endpoint's terminal outcome**, not by every endpoint receiving the record. Optional copies within a group retain the sender's existing semantics. Therefore, destinations that must **each** receive a record must be separate selected groups, even if they share a host or credentials. This PR validates the primary but does not enforce one reliable endpoint per group.

The default path uses **one sender and one endpoint set for all logs**. Its main endpoint is reliable; additional endpoints are reliable by default and can be configured as unreliable. It uses the same any-one-reliable sender behavior, without per-record group selection or independent group obligations. The routed groups and selection are supplied in code; `logs_config.routing.max_groups` limits the number of groups but does not define a routing policy.

Retryable outages and full queues do not create overflow drops for selected groups. Existing intentional exclusions, terminal loss policies (including permanent HTTP rejection), and credential behavior still apply. See [the mechanism design](logs-routing-mechanism.md) for limits and lifecycle details.
