---
slug: container-collect-all-startup-race
focus: "8 — Lifecycle Transitions"
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# Property: container-collect-all-startup-race

## What led to this property

sut-analysis.md §10 identifies the `container_collect_all` startup race: when
this config is enabled, containers that do not have a matching annotated source
are collected under a generic "container_collect_all" source. However,
autodiscovery processes container annotations and the `container_collect_all`
fallback non-atomically and in a specific ordering:

From the README.md (pkg/logs/README.md:84-86):
> When `logs_config.container_collect_all` is enabled, the launcher also starts
> a tailer for any docker container that does not have a corresponding source
> and which does not have autodiscovery-related labels. The logs-agent delays
> startup of the `container_collect_all` support until after autodiscovery has
> scanned its configuration sources once...

And from `comp/core/autodiscovery/providers/container.go:241-246`:
> container_collect_all configs must be added after configs generated from
> annotations, since services are reconciled against configs one-by-one
> instead of as a set, so if a container_collect_all config appears before an
> annotation one, it'll cause a logs config to be scheduled as
> container_collect_all, unscheduled, and then re-scheduled correctly.

This documents an *intentional mitigation* for a known ordering hazard. The
mitigation works when AD completes its initial scan before any container
produces logs. Under fault injection (AD startup latency, CPU throttle), the
window between "container first produces logs" and "AD delivers the annotated
source" is widened. During this window, the container's logs are attributed to
the generic `container_collect_all` source, with no metadata tags. When the
annotated source arrives, the old tailer is unscheduled (dropping the
`container_collect_all` source) and a new tailer is scheduled with the correct
metadata.

The race produces:
1. **Wrong-metadata log lines** — lines emitted before AD delivers the annotation
   are tagged with the generic source, missing service, env, version tags.
2. **Log gap** — during the unschedule/reschedule transition, there is a brief
   moment where neither tailer is active. Lines written during this gap are not
   collected.
3. **Duplicate logs** — the new tailer starts at the auditor's last offset for
   the container (if a registry entry exists). If the `container_collect_all`
   tailer advanced the offset, the annotated tailer picks up from that offset.
   If not (e.g., the identifier collision described in sut-analysis.md §9 item 6),
   the new tailer may re-read from an earlier position.

## Key code locations

- `comp/core/autodiscovery/providers/container.go:241-248` — ordering mitigation.
- `comp/core/autodiscovery/listeners/service.go:246-253` — service listener
  checks `container_collect_all` config.
- `pkg/logs/sources/source.go:152-157` — `container_collect_all` byte-count
  reporting to parent source.
- `pkg/logs/launchers/container/launcher.go:44-70` — Launcher struct,
  `addedSourcesDone`/`removedSourcesDone` channels for unblock on Stop.

## What fault triggers it

**AD startup latency** — delaying the autodiscovery provider's initial scan
(Antithesis CPU throttle on the AD goroutine, or a network fault to the container
metadata API) widens the window during which `container_collect_all` is the active
source.

**CPU throttling during unschedule/reschedule** — pausing the scheduler goroutine
between RemoveSource and AddSource for the annotated container widens the gap
window.

## Why it matters

Containers scheduled early in agent startup (before AD has completed its first
scan) receive wrong metadata for their initial log lines. In high-throughput
services, even a few seconds of wrong-metadata logs can represent thousands of
lines, all incorrectly attributed to `container_collect_all` in the Datadog
backend. This is invisible without comparing the metadata profile before and after
the transition.

## Assertions needed (all net-new SUT instrumentation)

1. **`Sometimes(container tailer transitioned from container_collect_all to annotated source)`**
   — SUT-side in the container launcher: when a source with name
   `container_collect_all` is removed and a new source for the same container
   with annotated metadata is added, emit `Sometimes(true)`. Confirms the race
   window was actually exercised.
2. **`Always(no log lines with wrong-metadata source appear in fakeintake after annotated source is active)`**
   — workload-side: after the annotated source has been confirmed active (by
   checking agent status or a known-tagged log line appearing), fakeintake should
   not receive new lines for that container tagged with the generic source.
3. **`Reachable(log gap detected: container tailer stopped, no new tailer active for container)`**
   — SUT-side: when `RemoveSource` is called for a container source and no
   replacement source has yet been added, emit `Reachable`. This marks the gap
   window as observed.

## Recovery window requirement

No network fault required for the startup-race property; only startup timing
(CPU throttle on AD goroutine) is needed. Fault-quiet window after AD delivers
annotations to confirm no further wrong-metadata lines appear.

## Open questions

- Does the container launcher for file-based container log collection (docker
  container use_file) have the same startup race, or only the Docker API tailer
  path? The AD ordering mitigation is in the `providers/container.go` which feeds
  both paths. `(needs human input)`
- Does the `container_collect_all` feature interact with the journald launcher
  (for systemd-based containers)? Not currently evident from code reading.
  `(needs human input)`
- What `Identifier()` value does the generic-source tailer use vs. the
  annotated-source tailer for the same container file path? If they differ,
  there is no offset-level collision but there may be a gap or overlap. If they
  are the same, the `container-identifier-no-collision` scenario applies here.
- Is the AD delay (`container_collect_all` startup delay) configurable? Setting
  it to 0 would guarantee the race window is exercised on every run with
  annotated containers.
- Does the `container_collect_all` path apply to Kubernetes pod logs (file
  tailing mode) or only to Docker API mode? `(needs human input)`

### Investigation Log

#### Does `container_collect_all` default value make this property vacuously unreachable in default config?

- Examined: `pkg/config/setup/common_settings.go:1765`.
- Found: `config.BindEnvAndSetDefault("logs_config.container_collect_all", false)`.
  Default is `false`.
- Conclusion: property is vacuously unreachable in default-config test runs.
  Test topology must explicitly set `logs_config.container_collect_all: true`.
  Existing `(partial:)` tag confirmed — removing it since question is fully answered.

#### Where is the AD startup-ordering mitigation implemented?

- Examined: `comp/core/autodiscovery/providers/container.go:241-248`,
  `comp/core/autodiscovery/common/utils/container_collect_all.go:14-25`,
  `comp/core/autodiscovery/listeners/service.go:246-253`.
- Found: the mitigation is in `providers/container.go` at the `Collect()` method.
  For each container entity, annotation-derived configs (`ExtractTemplatesFromAnnotations`)
  are appended to `c` first; then `AddContainerCollectAllConfigs(c, containerEntity)`
  appends the `container_collect_all` config *after* annotation configs. This ensures
  the AD scheduler sees annotation configs before the fallback, so the
  "unschedule+reschedule" scenario is avoided for the common case where annotation
  is present at first scan.
  In `listeners/service.go:246-253`, if `container_collect_all` is enabled, the
  `container_collect_all` config name is checked and the service listener suppresses
  it if an annotated config already exists.
- Conclusion: the mitigation is a best-effort ordering guarantee within a single
  `Collect()` call. It breaks if the AD provider's goroutine is CPU-throttled
  between first-scan completion and annotation delivery, or if a container starts
  between two `Collect()` calls. Antithesis can widen this window by pausing the
  AD goroutine.

## Merged-in evidence (from collect-all-metadata-correct)

The secondary file provided additional **code-level and scenario detail** for the
race window, and surfaced the `Services` store interaction:

**`Services.AddService` extends the race window:** the AD scheduler delivers
service events via `Services.AddService` (`services.go:34-45`), which holds
`s.mu` while doing a blocking channel send to all subscribers. If a subscriber
is CPU-throttled, `AddService` blocks while holding the mutex. `RemoveService`
for the generic source cannot proceed (also needs `s.mu`). This extends the
window during which the generic tailer is active and emitting wrong-metadata
logs beyond just the "AD startup delay."

**Auditor / identifier interaction (from secondary):**
During the tailer switch (generic → annotated), if the generic source and
annotated source produce different `Identifier()` values:
- Generic tailer's offset is stored under the generic identifier.
- Annotated tailer starts from its own identifier's registry entry (possibly 0
  or end-of-file per tailing mode).
- Logs between the generic tailer's last-acked offset and the annotated tailer's
  start position may be re-sent (duplicate) or lost depending on tailing mode
  and timing.
If identifiers are the same (both use the file path), the
`container-identifier-no-collision` collision scenario applies directly.

**Key additional code paths (from secondary):**
- `pkg/logs/service/services.go:34-45` — `AddService` blocking send under mutex
- `pkg/logs/service/services.go:48-65` — `RemoveService` also blocking under mutex
- `pkg/logs/schedulers/ad/scheduler.go:228-296` — AD scheduler source creation
- `pkg/logs/launchers/container/launcher.go:142-173` — `startSource` / `stopSource`

**Refined workload-side assertion (from secondary):**
```
For every log line received from known container C:
  if C has a known annotated source definition,
  assert line.tags contains service=<annotated_service>
    OR line.ingestionTimestamp < annotated_source_start_time
```
The time-based exception covers the startup window; the key property is that
once the annotated source is registered, no further wrong-metadata logs arrive.
