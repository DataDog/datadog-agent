# pkg/cloudfoundry

## Purpose

`pkg/cloudfoundry` contains Cloud Foundry integration helpers for the Datadog Agent. Its single sub-package, `containertagger`, is responsible for injecting host-level and CAPI (Cloud Foundry API) application metadata tags into running CF containers so that metrics emitted by those containers are enriched with the correct Datadog tags (e.g. `app_name`, `space_name`, `org_name`, `app_instance_guid`).

The package bridges two CF-specific subsystems:

- **Garden** – the CF container runtime API, used to run a shell script inside each container.
- **WorkloadMeta** – the agent's internal store of running container entities and their collector-assigned tags.

## Key elements

### `pkg/cloudfoundry/containertagger`

| Symbol | Kind | Description |
|--------|------|-------------|
| `ContainerTagger` | struct | Main type. Subscribes to WorkloadMeta container events and, for each new/updated container, calls into the Garden API to run a tag-injection script inside that container. |
| `NewContainerTagger(wmeta workloadmeta.Component) (*ContainerTagger, error)` | constructor | Obtains the Garden utility via `cloudfoundry.GetGardenUtil()` and reads `cloud_foundry_container_tagger.retry_count` / `retry_interval` from the agent config. |
| `(*ContainerTagger).Start(ctx context.Context)` | method | Launches a goroutine that subscribes to `workloadmeta` events (`SourceClusterOrchestrator`, `KindContainer`) and dispatches to `processEvent`. Cancel the context to stop. |

#### Internal behaviour

1. On `EventTypeSet` for a container:
   - Collects system and GCP host tags from `comp/metadata/host/hostimpl/hosttags`.
   - Appends them to the container's `CollectorTags` (which already contain CF application metadata populated by the WorkloadMeta collector).
   - Hashes the combined tag set and skips injection if the hash was already seen (deduplication).
   - Runs `/home/vcap/app/.datadog/scripts/update_agent_config.sh` inside the container via the Garden `Run` API, passing `DD_NODE_AGENT_TAGS=<comma-separated tags>` as an environment variable. The shell interpreter path is read from `cloud_foundry_container_tagger.shell_path`.
   - Retries up to `retry_count` times with `retry_interval` delay on failure.
2. On `EventTypeUnset`, removes the container's tag hash from the seen-set so resources are reclaimed.

#### Configuration keys

| Key | Default behaviour |
|-----|-------------------|
| `cloud_foundry_container_tagger.retry_count` | Number of times to retry tag injection |
| `cloud_foundry_container_tagger.retry_interval` | Seconds between retries |
| `cloud_foundry_container_tagger.shell_path` | Path to the shell used inside the container |

## Usage

The package is consumed by the fx component at `comp/agent/cloudfoundrycontainer/cloudfoundrycontainerimpl`. That component checks `env.IsFeaturePresent(env.CloudFoundry)` and the `cloud_foundry_buildpack` config flag before creating and starting the tagger:

```go
// comp/agent/cloudfoundrycontainer/cloudfoundrycontainerimpl/cloudfoundrycontainer.go
if env.IsFeaturePresent(env.CloudFoundry) && !deps.Config.GetBool("cloud_foundry_buildpack") {
    containerTagger, err := cloudfoundrycontainertagger.NewContainerTagger(deps.WMeta)
    // ...
    containerTagger.Start(ctx)
}
```

The component is wired into the main agent binary. It does not run in the buildpack flavour of the CF agent because buildpack containers manage their own tag injection differently.

---

## Logging

All log output from `containertagger` goes through `pkg/util/log` (imported directly as `log`). The package uses:

- `log.Infof` — on successful tag injection and component startup.
- `log.Warnf` — on event processing errors and failed injection attempts.
- `log.Debugf` — for per-event and per-attempt debug traces.

Because `containertagger` is a `pkg/` package (not an fx component), it imports `pkg/util/log` directly rather than declaring a `comp/core/log/def.Component` dependency. See [pkg/util/log docs](util/log.md) for the full API and rate-limiting helpers.

---

## Related components

- **`comp/core/workloadmeta`** — `ContainerTagger.Start` subscribes to the store using `SourceClusterOrchestrator` + `KindContainer` filter. Each `EventTypeSet` bundle delivers a snapshot of live CF containers with their `CollectorTags` already populated by the workloadmeta CloudFoundry collector. See [comp/core/workloadmeta docs](../comp/core/workloadmeta.md).
- **`pkg/util/log`** — used for all diagnostic output. The `log.Warnf` return-value pattern (returns the warning as an `error`) is used in `processEvent`. See [pkg/util/log docs](util/log.md).
- **`comp/metadata/host/hostimpl/hosttags`** — supplies system and GCP host tags that are merged with container tags before injection.
