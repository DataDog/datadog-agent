Turn budget wrap-up was requested after 4 assistant turns (soft limit 4, grace 1). Process-mode live steering is unavailable, so the child was warned at launch to wrap up by this budget. Output may be partial.

## Review

### Top 3 observations

1. **Blocker — the proposed A/B pair cannot safely share one scenario checkout when `run-episode` overlaps.**  
   `episode-ctl` writes mutable, fixed paths under the supplied scenario directory: `run-result.json` and `results/<episode>.json` (with an archive step for pre-existing results). The CLI does not offer result/output-path overrides. Concurrent control and treatment invocations using the same episode name can overwrite or archive one another’s evidence, invalidating both result and archive provenance.  
   **Evidence:** `gensim/src/controller/README.md:110-130` documents the fixed `run-result.json` location; `gensim/src/controller/controller/controller.py:1080-1098` derives it as `<scenario>/run-result.json`; `gensim/src/controller/controller/controller.py:1240-1328` reads the episode result from the scenario directory.  
   **Minimal change:** Materialize a read-only, content-hashed source bundle plus one separate per-arm working directory (or separate source checkout) before deployment. Treat `arm_workdir`, result file path, and copied result checksum as required manifest fields. This is smaller and safer than modifying `episode-ctl` to add output-path plumbing.

2. **High — “max builds=1” is not actually enforced by a controller that only serializes two `episode-ctl deploy` processes.**  
   A single `deploy` fan-outs *all* buildable services through a `ThreadPoolExecutor(max_workers=len(builds))`, and Cloud Build submits each service asynchronously before polling completion. Therefore one arm’s deploy can consume several Cloud Build slots; a campaign-level `max builds=1` lock only limits deploy commands, not Cloud Build operations. Cloud Build IDs are parsed only in the process and logged; the public deploy command returns only success/failure and namespace/env text.  
   **Evidence:** `gensim/src/controller/controller/controller.py:716-744` selects Cloud Build; `:840-963` constructs an async submission per service and parses IDs; `:965-966` submits all services in parallel; `gensim/src/controller/controller/cli.py:76-109` exposes no build-concurrency or build-ID output option.  
   **Minimal change:** Define the initial limit as **one active deploy**, not “one build,” and record Cloud Build IDs by capturing structured controller logs during that deploy. If the hard requirement is truly one Cloud Build at a time, add a narrow `--build-parallelism=1` flag/internal parameter and test it; do not claim the existing CLI supplies that guarantee.

3. **High — treatment identity is unsupported by the documented CLI contract unless control and treatment have independently rendered scenario bundles.**  
   `deploy` accepts only scenario path, namespace, env, timeout, and org at the CLI surface. It does not expose `skip_build`, registry/image digest selection, Helm value overrides, Agent image replacement, or a feature-toggle argument. Although `EpisodeController.deploy` has internal `skip_build`, `helm_values`, and `image_registry` parameters, `cmd_deploy` does not pass them from CLI options. The plan must not assume the remote controller can set and attest an Agent digest/toggle per arm merely by invoking the published commands.  
   **Evidence:** `gensim/src/controller/controller/controller.py:1114-1139` exposes those controls only as Python method parameters; `gensim/src/controller/controller/cli.py:76-109` exposes none of them. The rendered values carry `datadog.env` and a registry prefix, not immutable per-service digest mappings (`controller.py:1186-1207`).  
   **Minimal change:** Freeze and hash separately rendered control/treatment bundles before the campaign, with the desired Agent image/toggle already present in each bundle. The controller should deploy these immutable arm bundles using only existing flags. Before implementation, perform a one-arm qualification that reads the running Agent container `imageID` and proves it equals the intended digest; reject a scored pair if it does not.

### Overall assessment

The phase ordering fits existing `episode-ctl` capabilities: separate `deploy`, `run-episode`, `verify`, `archive`, and `teardown` commands exist, and explicit namespace/env arguments support isolated arms (`gensim/src/controller/README.md:36-105`). The durable remote-controller proposal is viable only as a deliberately thin wrapper around those commands, after adding per-arm work directories and correcting the unsupported build/digest assumptions; fleet remains correctly out of scope because the documented worker fan-out method is presently a stub (`docs/dev/gensim-global-1hz-scaled-experiment-plan.md:481-486`).

### One thing other reviewers will miss

`episode-ctl deploy` names the Helm release from the sanitized **scenario directory basename**, not from namespace or arm identity (`gensim/src/controller/controller/controller.py:1144-1147`, `:1220-1227`). Separate namespaces prevent release collisions, but per-arm bundle directories need deterministic, Helm-valid **different basenames** (for example `scenario-control-<hash>` and `scenario-treatment-<hash>`). Otherwise source-bundle identity may be ambiguous in Helm history and logs even though Kubernetes isolation works.

### Recommended lowest-edit implementation

- Use a remote scheduled job/runner with durable object storage, not a new service/API or fleet workflow.
- Pre-render and hash two immutable arm directories, give each a unique namespace/DD_ENV and unique directory basename.
- Serialize all deploys for a same-scenario pair; regard one deploy as the only currently enforceable build unit.
- Capture command stdout/stderr and explicitly snapshot Kubernetes pod/container `imageID` values after readiness; copy results immediately from each arm workdir before verification/archive.
- Persist only durable checkpoints needed for command idempotency: bundle prepared, deploy command started/completed, readiness snapshot, episode started/completed, result copied/checksummed, verified, archive completed/checksummed, teardown observed. On restart, reconcile namespaces, Helm releases, pod image IDs, and result/archive object existence before selecting the next action.
- Avoid implementing custom build queues, a fleet dispatcher, generic Helm patching, or a duplicate episode executor until an actual CLI contract requires them.