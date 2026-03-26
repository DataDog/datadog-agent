> **TL;DR:** `pkg/languagedetection` detects the programming language of running processes in two tiers — unprivileged command-line classification and privileged ELF/interpreter analysis — to support APM auto-instrumentation library injection and workload metadata enrichment.

# pkg/languagedetection

## Purpose

`pkg/languagedetection` detects the programming language of running processes
so the Datadog Agent can attach language metadata to APM traces, container
annotations, and workload metadata. The detection result is used by the
library-injection feature (Admission Controller) to decide which APM
auto-instrumentation library to inject.

Detection works in two tiers:

1. **Unprivileged (process agent / node agent):** inspects the process command
   line and executable name. Works anywhere without elevated rights.
2. **Privileged (system-probe, Linux only):** reads the process binary from
   `/proc/<pid>/exe` and looks for language-specific signatures. Requires root
   or `CAP_PTRACE`.

---

## Key elements

### Key types

#### `pkg/languagedetection` (root package)

| Symbol | Description |
|--------|-------------|
| `DetectLanguage(procs []Process, sysprobeConfig)` | Main entry point. Runs unprivileged detection for all processes; for those that remain `Unknown`, optionally delegates to the system-probe via HTTP (`/language_detection/detect`). Returns `[]*Language` parallel to the input slice. |
| `languageNameFromCommand(command)` | Pure-string classifier: exact matches (`python`, `java`, `node`, …) and prefix matches with optional validators (e.g. `ruby2.7`, `php8`). |
| `knownPrefixes` / `exactMatches` | Package-level maps that define the classification rules used by `languageNameFromCommand`. |
| `cliDetectors` | Slice of `Detector` implementations run before command-name matching. Currently contains `JRubyDetector` (detects JRuby from the JVM command line). |

#### `languagemodels/` — shared types

| Symbol | Description |
|--------|-------------|
| `LanguageName` | String enum: `Go`, `Python`, `Java`, `Ruby`, `Node`, `Dotnet`, `PHP`, `CPP`, `Unknown`. |
| `Language` | `{ Name LanguageName; Version string }` — result of a single detection. |
| `Detector` | Interface: `DetectLanguage(Process) (Language, error)`. Implemented by all detectors. |
| `Process` | Interface: `GetPid() int32`, `GetCommand() string`, `GetCmdline() []string`. Adapters in the process agent and workloadmeta collector implement this. |
| `LanguageSet` | `map[LanguageName]struct{}` — deduplicated set of languages for a container. Serialises to proto with `ToProto()`. |
| `TimedLanguageSet` | `map[LanguageName]time.Time` — same as `LanguageSet` but each entry carries a TTL expiration. Used by the cluster agent handler to age-out stale languages. |
| `Container` | `{ Name string; Init bool }` — identifies a pod container (regular or init). |
| `ContainersLanguages` | `map[Container]LanguageSet` — per-container language sets. Converts to Kubernetes annotations via `ToAnnotations()` and to proto via `ToProto()`. |
| `TimedContainersLanguages` | `map[Container]TimedLanguageSet` — timed variant; supports `Merge`, `RemoveExpiredLanguages`, `EqualTo`. |
| `AnnotationPrefix` | `"internal.dd.datadoghq.com/"` — prefix for language detection annotations on Kubernetes resources. |
| `GetLanguageAnnotationKey(containerName)` | Builds the full annotation key for a container (e.g. `internal.dd.datadoghq.com/my-container.detected_langs`). |
| `AnnotationRegex` | Regex that matches and parses language detection annotation keys, extracting container name and init-container flag. |

### Key functions

#### `privileged/` — binary analysis (Linux, system-probe)

| Symbol | Description |
|--------|-------------|
| `LanguageDetector` | Struct used by system-probe. Holds a binary-identity LRU cache (size 1000, keyed by device+inode) to avoid re-analysing the same binary. |
| `NewLanguageDetector()` | Constructor; wires the four privileged detectors: `TracerDetector`, `InjectorDetector`, `GoDetector`, `DotnetDetector`. |
| `DetectWithPrivileges(procs)` | Runs each detector in order; returns on first non-Unknown result per process. Results are cached by binary identity. |
| `detectorsWithPrivilege` | Package-level slice of `Detector` implementations that require elevated access (binary inspection). |

The privileged detectors live in
`internal/detectors/privileged/` and are not part of the public API:

- **GoDetector** — looks for the Go build-info section in the ELF binary.
- **DotnetDetector** — checks PE/ELF headers or runtime signatures.
- **TracerDetector** — detects a running APM tracer (e.g. dd-trace-java jar in
  the class path).
- **InjectorDetector** — detects that the library injector has already run.

### Configuration and build flags

The privileged detection tier is Linux-only and requires root or `CAP_PTRACE`. Unprivileged detection compiles on all platforms. The `system_probe_config.language_detection.enabled` key gates the fallback HTTP call from `DetectLanguage` to system-probe.

#### `util/` — Kubernetes owner utilities

| Symbol | Description |
|--------|-------------|
| `NamespacedOwnerReference` | Identifies a Kubernetes owner (Kind, Name, Namespace, APIVersion). |
| `GetNamespacedBaseOwnerReference(podDetails)` | Returns the "base" owner of a pod. If the immediate owner is a `ReplicaSet`, it walks up to the parent `Deployment` (via `kubernetes.ParseDeploymentForReplicaSet`). |
| `GetGVR(ownerRef)` | Converts an owner reference to a `schema.GroupVersionResource` for use with the dynamic Kubernetes client. |
| `SupportedBaseOwners` | Set of owner kinds that the language detection feature supports (currently only `Deployment`). |

---

## Usage

### Unprivileged detection (process agent / node agent)

```go
import "github.com/DataDog/datadog-agent/pkg/languagedetection"

langs := languagedetection.DetectLanguage(processes, sysprobeConfig)
for i, lang := range langs {
    fmt.Printf("pid %d: %s %s\n", processes[i].GetPid(), lang.Name, lang.Version)
}
```

When `system_probe_config.language_detection.enabled` is `true` and the OS is
Linux, `DetectLanguage` automatically falls back to the system-probe HTTP
endpoint for processes whose language could not be determined from the command
line alone.

### Privileged detection (system-probe)

```go
import "github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"

detector := privileged.NewLanguageDetector()
langs := detector.DetectWithPrivileges(processes)
```

This runs only on Linux and requires the system-probe to be root (or have
`CAP_PTRACE`). A single permission-denied warning is emitted via `sync.Once`
to avoid log spam.

### Kubernetes annotation flow

The cluster agent uses `ContainersLanguages.ToAnnotations()` to produce
annotations like:

```
internal.dd.datadoghq.com/my-container.detected_langs: java,python
internal.dd.datadoghq.com/init.init-container.detected_langs: python
```

These are patched onto Deployment (or other owner) objects so the Admission
Controller can inject the right libraries at pod creation time.

### Importers

| Component | How it uses the package |
|-----------|-------------------------|
| `comp/core/workloadmeta/collectors/internal/process` | Calls `DetectLanguage` to annotate process entities in workloadmeta. |
| `pkg/process/metadata/workloadmeta/extractor` | Extracts language info from workloadmeta process entities. |
| `pkg/discovery/language` | Thin Linux wrapper that calls `DetectLanguage` for the discovery service. |
| `cmd/system-probe` (language detection module) | Hosts `LanguageDetector.DetectWithPrivileges` behind an HTTP endpoint. |

---

## Cross-references

| Related package / component | Relationship |
|-----------------------------|--------------|
| [`pkg/discovery`](../discovery/discovery.md) | The `pkg/discovery` module runs inside system-probe and is the primary production caller of `privileged.LanguageDetector`. `discovery.module.handleServices` creates a `privileged.NewLanguageDetector()` and calls `DetectWithPrivileges` for each listening process. The `discovery/language` sub-package mirrors the `LanguageName` constants from `languagemodels` into the `language.Language` string type consumed by the `model.Service` wire type. |
| [`comp/languagedetection/client`](../../comp/languagedetection/client.md) | The language-detection client component runs inside the **node agent** (process-agent role). It subscribes to `KindProcess` and `KindKubernetesPod` workloadmeta events, accumulates per-container language data obtained from `DetectLanguage`, and streams it to the Cluster Agent via `PostLanguageMetadata`. The `ContainersLanguages` / `TimedContainersLanguages` types from `languagemodels` are the wire types that flow between this package and the client component. |
| [`pkg/clusteragent`](../clusteragent/clusteragent.md) | `pkg/clusteragent/languagedetection` (a sub-package of the Cluster Agent) receives the `ParentLanguageAnnotationRequest` proto sent by the client component and writes the result back to Kubernetes resource annotations using `ContainersLanguages.ToAnnotations()`. The `mutate/autoinstrumentation` admission webhook then reads those annotations to decide which APM library to inject. |
| [`pkg/process/monitor`](monitor.md) | The process monitor singleton (`GetProcessMonitor`) delivers exec/exit notifications that trigger language detection re-runs. The workloadmeta process collector subscribes to exec events via `ProcessMonitor.SubscribeExec` to schedule `DetectLanguage` calls for newly started processes, avoiding a polling loop. |
| [`comp/core/workloadmeta`](../../comp/core/workloadmeta.md) | The process collector inside workloadmeta (`collectors/internal/process`) is the bridge between this library and the rest of the agent. It holds a `DetectLanguage` reference, annotates `workloadmeta.Process` entities with language metadata, and publishes `EventTypeSet` events that the language-detection client component consumes. |

### End-to-end data flow

```
[process exec event]
    |
    v
pkg/process/monitor.SubscribeExec
    |
    v
workloadmeta process collector
    ├─> languagedetection.DetectLanguage (unprivileged: command-line)
    │       └─> falls back to system-probe HTTP if unknown
    │               └─> privileged.LanguageDetector.DetectWithPrivileges (ELF/interpreter)
    └─> workloadmeta.Process entity (Language field set)
            |
            v
comp/languagedetection/client
    └─> PostLanguageMetadata → Cluster Agent
            |
            v
    pkg/clusteragent/languagedetection
        └─> ContainersLanguages.ToAnnotations() → Kubernetes Deployment annotations
                |
                v
        mutate/autoinstrumentation webhook
            └─> APM library injected at pod creation
```
