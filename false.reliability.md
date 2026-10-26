Turn budget wrap-up was requested after 4 assistant turns (soft limit 4, grace 1). Process-mode live steering is unavailable, so the child was warned at launch to wrap up by this budget. Output may be partial.

## Review

### Top 3 observations

1. **Blocker — the proposed log/lock does not yet define an atomic, idempotent external-action boundary.**  
   **Location:** `/var/folders/27/n33yr1b10j7bxg5ttzw6n_ch0000gp/T/pi-subagent-f0uPKE/task.md:1`; `docs/dev/gensim-global-1hz-scaled-experiment-plan.md:278`  
   A controller crash can occur after `episode-ctl deploy` or `run-episode` has been accepted but before its Cloud Build/deployment/execution identity is durably recorded. Re-running the command may create a second build or episode, while a campaign lock alone cannot establish which external action won. The existing guide explicitly requires retaining idempotency keys and service-returned execution/workflow IDs *before retrying an ambiguous request* (`docs/dev/gensim-global-1hz-scaled-experiment-plan.md:278`).

   **Minimal change:** make every side-effecting transition a write-ahead operation with a stable `campaign_id/pair_id/arm_id/attempt_id`, then reconcile by that identity before issuing another request:
   - durably append `DEPLOY_INTENT` before invoking deploy;
   - record command correlation data (namespace, release name, labels, requested build inputs) immediately;
   - on restart, discover Cloud Build, Helm/Kubernetes, and episode resources by those immutable labels/identities;
   - only append `DEPLOY_CONFIRMED` / `EPISODE_CONFIRMED` after an observed remote identity is persisted;
   - classify a request as `AMBIGUOUS` rather than retrying if reconciliation cannot prove it did not start.

   Required durable terminal transitions should include `PRECHECKED`, `CONTROL_DEPLOY_INTENT`, `CONTROL_READY`, `TREATMENT_DEPLOY_INTENT`, `TREATMENT_READY`, `EPISODE_INTENT`/`EPISODE_OBSERVED` for each arm, `VERIFYING`, `ARCHIVE_CONFIRMED`, `CLEANUP_INTENT`, `CLEANUP_CONFIRMED`, and `TERMINAL_SUCCESS|FAILED|MANUAL_RECONCILIATION`. The append-only log should derive the current state; it must not be only audit output.

2. **High — resource limits need persisted leases, deadlines, and reconciliation against actual remote consumption; otherwise crashes can leak capacity or make the controller wait forever.**  
   **Location:** `/var/folders/27/n33yr1b10j7bxg5ttzw6n_ch0000gp/T/pi-subagent-f0uPKE/task.md:1`; `docs/dev/gensim-global-1hz-scaled-experiment-plan.md:451-455`, `472-481`  
   `max builds=1`, `active pairs=1`, and `running episodes=2` are appropriate initial limits, but they are incomplete if held in process memory or released when the CLI returns. A crashed controller may leave a Cloud Build, deployment, or episode running while a restarted controller either oversubscribes quotas or deadlocks on a leaked token. The source guidance confirms that normal deploys rebuild and push mutable scenario/service tags, and concurrent same-scenario build/deploy can race (`docs/dev/gensim-global-1hz-scaled-experiment-plan.md:451-455`).

   **Minimal change:** persist capacity leases with owner attempt, acquisition time, expiry/heartbeat, and the remote IDs that prove consumption. On restart, reconstruct permits from remote reality—not merely from manifest state—and release a permit only after the remote build/episode is terminal or its cancellation/cleanup is observed. Add bounded admission queues, per-stage deadlines, exponential backoff with a maximum attempt/deadline budget, and an explicit `capacity_unknown` stop state. Readiness, archive, and cleanup waits must have deadlines; timeout should transition to reconciliation/cancellation, not loop indefinitely. Do not start any later pair while the sole active pair is ambiguous or has unconfirmed cleanup.

3. **High — failure containment and cleanup criteria are underspecified for asymmetric or archival failures.**  
   **Location:** `/var/folders/27/n33yr1b10j7bxg5ttzw6n_ch0000gp/T/pi-subagent-f0uPKE/task.md:1`; `docs/dev/gensim-global-1hz-scaled-experiment-plan.md:199-206`, `290`  
   If control becomes ready but treatment deployment/readiness fails, or one concurrent `run-episode` fails/times out, the plan must prevent the healthy arm from continuing into an unpaired capture. Likewise, an archive upload/checksum failure must not prevent teardown and leak an environment. The documented local path requires unique namespace/environment identities, runtime Agent validation, and verification of namespace, monitors, Helm release, and cluster-scoped RBAC cleanup after interruption (`docs/dev/gensim-global-1hz-scaled-experiment-plan.md:199-206`). A ramp also cannot advance until expected namespaces/vclusters are gone, archives are complete, and monitor/RBAC leaks are absent (`:290`).

   **Minimal change:** establish a single failure-containment policy:
   - before either episode begins, a failed or timed-out deployment cancels/tears down the other deployed arm;
   - once either episode action is observed, do **not** retry either arm; stop/cancel the counterpart where supported, retain the failed paired attempt, and mark it terminally non-retryable;
   - execute cleanup in a durable `finally` path independent of archive success, retaining partial evidence locally/object-store first;
   - scope deletion exclusively by immutable campaign/pair/arm labels and exact namespace/release identities—never broad name-prefix cleanup;
   - require observed deletion of namespace, Helm release, monitors, and cluster-scoped RBAC before `CLEANUP_CONFIRMED`; otherwise retain the lease and escalate for manual cleanup.

### Overall assessment

The proposed episode-ctl path is viable for a deliberately single-pair campaign, and the initial concurrency bounds plus unique namespace/DD_ENV are sound starting safeguards. It is not durable enough to implement until ambiguous external requests, remote-backed capacity leases, and asymmetric-failure cleanup are made explicit state-machine contracts.

### One thing other reviewers will miss

**Mutable workload tags remain a runtime integrity risk even with sequential build/deploy.** Recording runtime pod image IDs is evidence after the fact, but the plan must make digest equality a launch/verification gate. The first arm can resolve a mutable tag differently from the second arm—or be affected by later tag movement—despite serialized builds. The published guide requires an end-to-end content-addressed image contract and says to fail when deployments resolve different workload image bytes (`docs/dev/gensim-global-1hz-scaled-experiment-plan.md:451-455`). Until that exists, reconcile and compare the full service→runtime-image-digest map for both ready arms before either `run-episode`; if unequal, tear both down and terminally reject the pair rather than treating it as a retryable infrastructure failure.