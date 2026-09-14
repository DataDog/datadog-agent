# Upstream OpenTelemetry Operator auto-instrumentation — factual reference

**Purpose.** Factual description of how the community OpenTelemetry Operator implements
auto-instrumentation injection, to be used as a requirements input for compatibility work in the
Datadog Cluster Agent. This document describes only what upstream does; it contains no proposals.

**Source of truth.** `github.com/open-telemetry/opentelemetry-operator`, `main` @
`843b527` (2026-09-01). All the injection files were byte-compared against tag `v0.158.0`
(`internal/instrumentation/{javaagent,python,nodejs,dotnet,podmutator,annotation}.go` are
identical), so everything below is released behaviour, not unreleased `main` drift.
`versions.txt` on this commit reports `operator=0.158.0`.

**Note on file layout.** The instrumentation package moved from `pkg/instrumentation/` to
`internal/instrumentation/`. Paths below use the current location.

---

## 0. Where injection happens

| Item | Value | Source |
|---|---|---|
| Webhook path | `/mutate-v1-pod` | `internal/webhook/podmutation/webhookhandler.go:21` |
| Verbs / resources | `create` on `pods` (no `update`) | same |
| `failurePolicy` | `ignore` | same |
| `objectSelector` | none — every created pod in the cluster goes through the webhook | same |
| Namespace object | fetched with `client.Get` on `req.Namespace`, not from the pod | `webhookhandler.go` `Handle()` |
| Error behaviour | on any mutator error the response is `admission.Errored(500)` but with `res.Allowed = true` forced, so pod creation is never blocked | `webhookhandler.go` `Handle()` |

Entry point of the instrumentation mutator: `instPodMutator.Mutate`,
`internal/instrumentation/podmutator.go:210`.

### Idempotency guard (important)

`Mutate` bails out immediately if `isAutoInstrumentationInjected(pod)` returns true
(`internal/instrumentation/helper.go:34`). That function returns true if **either**:

- an init container is named one of `opentelemetry-auto-instrumentation-{dotnet,java,nodejs,python}`,
  `otel-agent-attach-apache`, `otel-agent-source-container-clone`; **or**
- a container named `opentelemetry-auto-instrumentation` exists (the Go sidecar); **or**
- **any** container that is not the collector container has an env var named
  `OTEL_RESOURCE_ATTRIBUTES_NODE_NAME`.

The last condition means a pod that already carries `OTEL_RESOURCE_ATTRIBUTES_NODE_NAME` for any
unrelated reason is silently skipped entirely.

---

## A. Annotation contract (targeting)

### A.1 — Recognized annotation keys

All keys are declared in `internal/instrumentation/annotation.go:12-35` (prefix
`instrumentation.opentelemetry.io/`).

| Annotation | Purpose |
|---|---|
| `instrumentation.opentelemetry.io/inject-java` | enable Java injection |
| `instrumentation.opentelemetry.io/inject-nodejs` | enable Node.js injection |
| `instrumentation.opentelemetry.io/inject-python` | enable Python injection |
| `instrumentation.opentelemetry.io/inject-dotnet` | enable .NET injection |
| `instrumentation.opentelemetry.io/inject-go` | enable Go injection |
| `instrumentation.opentelemetry.io/inject-apache-httpd` | enable Apache HTTPD injection |
| `instrumentation.opentelemetry.io/inject-nginx` | enable Nginx injection |
| `instrumentation.opentelemetry.io/inject-sdk` | env-vars-only injection (no agent) |
| `instrumentation.opentelemetry.io/container-names` | common container selection (all languages) |
| `instrumentation.opentelemetry.io/java-container-names` | per-language container selection |
| `instrumentation.opentelemetry.io/nodejs-container-names` | per-language container selection |
| `instrumentation.opentelemetry.io/python-container-names` | per-language container selection |
| `instrumentation.opentelemetry.io/dotnet-container-names` | per-language container selection |
| `instrumentation.opentelemetry.io/go-container-names` | per-language container selection |
| `instrumentation.opentelemetry.io/apache-httpd-container-names` | per-language container selection |
| `instrumentation.opentelemetry.io/inject-nginx-container-names` | per-language container selection (note the inconsistent `inject-` prefix — this is upstream's actual constant, `annotationInjectNginxContainersName`) |
| `instrumentation.opentelemetry.io/sdk-container-names` | per-language container selection |
| `instrumentation.opentelemetry.io/otel-python-platform` | `glibc` (default) or `musl`; selects the copy source in the Python init container |
| `instrumentation.opentelemetry.io/otel-dotnet-auto-runtime` | `linux-x64` (default) or `linux-musl-x64`; selects the CoreCLR profiler `.so` path |
| `instrumentation.opentelemetry.io/otel-go-auto-target-exe` | value of `OTEL_GO_AUTO_TARGET_EXE` on the Go sidecar |

Separately, `pkg/constants/env.go:19-25` defines
`instrumentation.opentelemetry.io/default-auto-instrumentation-<lang>-image` annotations, but those
are written by the operator **onto the `Instrumentation` CR** (not onto pods) and are used by the
auto-upgrade controller. They are not part of the pod targeting contract.

Resource-attribute annotations use a different prefix, `resource.opentelemetry.io/<attr>`
(`pkg/constants/env.go:28`) — see section C.12.

### A.2 — Value resolution semantics

Resolution happens in two steps.

**Step 1 — effective value**, `annotationValue(ns, pod, annotation)`,
`internal/instrumentation/annotation.go:38`:

```go
func annotationValue(ns, pod metav1.ObjectMeta, annotation string) string {
	podAnnValue := pod.Annotations[annotation]
	nsAnnValue := ns.Annotations[annotation]

	// if the namespace value is empty, the pod annotation should be used, whatever it is
	if nsAnnValue == "" {
		return podAnnValue
	}
	// if the pod value is empty, the annotation should be used (true, false, instance)
	if podAnnValue == "" {
		return nsAnnValue
	}
	// the pod annotation isn't empty -- if it's an instance name, or false, that's the decision
	if !strings.EqualFold(podAnnValue, "true") {
		return podAnnValue
	}
	// pod annotation is 'true', and if the namespace annotation is false, we just return 'true'
	if strings.EqualFold(nsAnnValue, "false") {
		return podAnnValue
	}
	// by now, the pod annotation is 'true', and the namespace annotation is either true or an
	// instance name so, the namespace annotation can be used
	return nsAnnValue
}
```

**Step 2 — CR lookup**, `instPodMutator.getInstrumentationInstance`,
`internal/instrumentation/podmutator.go:362`:

```go
	instValue := annotationValue(ns.ObjectMeta, pod.ObjectMeta, instAnnotation)

	if instValue == "" || strings.EqualFold(instValue, "false") {
		return nil, nil
	}

	if strings.EqualFold(instValue, "true") {
		if !pm.config.EnableInstrumentationCRDs {
			return &pm.config.Instrumentation, nil
		}
		return pm.selectInstrumentationInstanceFromNamespace(ctx, ns)
	}

	var instNamespacedName types.NamespacedName
	if instNamespace, instName, namespaced := strings.Cut(instValue, "/"); namespaced {
		instNamespacedName = types.NamespacedName{Name: instName, Namespace: instNamespace}
	} else {
		instNamespacedName = types.NamespacedName{Name: instValue, Namespace: ns.Name}
	}

	otelInst := &v1alpha1.Instrumentation{}
	err := pm.Client.Get(ctx, instNamespacedName, otelInst)
	if err != nil {
		return nil, err
	}
	return otelInst, nil
```

And `selectInstrumentationInstanceFromNamespace`, `internal/instrumentation/podmutator.go:392`:

```go
	var otelInsts v1alpha1.InstrumentationList
	if err := pm.Client.List(ctx, &otelInsts, client.InNamespace(ns.Name)); err != nil {
		return nil, err
	}
	switch s := len(otelInsts.Items); {
	case s == 0:
		return nil, errNoInstancesAvailable          // "no OpenTelemetry Instrumentation instances available"
	case s > 1:
		return nil, errMultipleInstancesPossible     // "multiple OpenTelemetry Instrumentation instances available, cannot determine which one to select"
	default:
		return &otelInsts.Items[0], nil
	}
```

Summary of the four value forms:

| Value | Behaviour |
|---|---|
| `""` (absent) | no injection for that language |
| `"false"` (case-insensitive) | no injection for that language |
| `"true"` (case-insensitive) | **List all `Instrumentation` CRs in the pod's namespace. There is no hardcoded default CR name. Exactly one must exist**: zero → error, more than one → error. |
| `"<name>"` | `Get` the `Instrumentation` named `<name>` in the **pod's** namespace |
| `"<namespace>/<name>"` | `Get` the `Instrumentation` named `<name>` in `<namespace>`. Split is on the **first** `/` (`strings.Cut`). |

Anything else (e.g. `"yes"`, `"1"`) is treated as a CR *name* and will produce a `NotFound`.

**Discrepancy with the rendered docs, flagged deliberately:**
<https://opentelemetry.io/docs/platforms/kubernetes/operator/automatic/> says `"true"` injects "the
`Instrumentation` resource with **default name** from the current namespace". That is inaccurate —
there is no default name; the code requires exactly one CR in the namespace. The in-repo doc
`docs/auto-instrumentation/README.md:67` is closer: "inject and `Instrumentation` resource from the
namespace".

**Error handling on lookup failure.** `Mutate` returns `(pod, err)` on any lookup error
(`podmutator.go:231-235` and the equivalent block for every language). The webhook handler converts
that into a 500 response with `Allowed = true`, so the pod is created **uninstrumented** and an
error is logged. It is not a hard failure.

**Operator-level kill switches.** Even when an annotation resolves, injection is dropped if the
corresponding operator flag is off (`podmutator.go:236-315`); the operator logs an error and emits a
`Warning` / `InstrumentationRequestRejected` event on the pod. Defaults from
`internal/config/config.go:188-195`:

| Language | Flag | Default |
|---|---|---|
| Java | `--enable-java-instrumentation` | `true` |
| Node.js | `--enable-nodejs-instrumentation` | `true` |
| Python | `--enable-python-instrumentation` | `true` |
| .NET | `--enable-dotnet-instrumentation` | `true` |
| Apache HTTPD | `--enable-apache-httpd-instrumentation` | `true` |
| Go | `--enable-go-instrumentation` | **`false`** |
| Nginx | `--enable-nginx-instrumentation` | **`false`** |
| (multi-instr.) | `--enable-multi-instrumentation` | `true` |

`inject-sdk` has no kill switch. `EnableInstrumentationCRDs` defaults to `true`
(`config.go:240`); when set to `false` the operator ignores the CRDs entirely and `"true"` resolves
to a single static `Instrumentation` embedded in the operator's own config file
(`config.go:149-152`).

### A.3 — Pod vs namespace precedence

Derived from `annotationValue` above, and confirmed by the table in
`internal/instrumentation/annotation_test.go` (`TestEffectiveAnnotationValue`):

| Pod annotation | Namespace annotation | Effective value | Note |
|---|---|---|---|
| absent | absent | `""` | no injection |
| absent | anything | namespace value | namespace applies to all pods |
| anything | absent | pod value | |
| `"false"` | `"true"` or `"<name>"` | **`"false"`** | pod wins — opt-out always honoured |
| `"<name>"` | `"true"` or `"<other>"` | **pod's `<name>`** | pod wins |
| `"true"` | `"false"` | **`"true"`** | pod wins — the pod re-enables what the namespace disabled |
| `"true"` | `"<name>"` | **namespace's `<name>`** | namespace wins |
| `"true"` | `"true"` | `"true"` | |

The one non-obvious rule: when the pod says `"true"`, the pod is only expressing "yes, instrument
me" — it delegates the *choice of CR* to the namespace. In every other case the pod value is final.
Comparisons are case-insensitive (`strings.EqualFold`).

### A.4 — Container restriction

Two mechanisms, resolved through the same `annotationValue` pod/namespace precedence.

**Common:** `instrumentation.opentelemetry.io/container-names`, a comma-separated list, applied to
*every* enabled language (`languageInstrumentations.setCommonInstrumentedContainers`,
`podmutator.go:128`).

Evaluated for every enabled language — but **only if at least one language is enabled**.
`setCommonInstrumentedContainers` runs *after* the `if !insts.hasAnyInstrumentation() { return pod, nil }`
guard in `Mutate`, so a pod carrying a malformed `container-names` value but no `inject-<lang>`
annotation is never validated and falls through untouched. An earlier revision of this document
said "always evaluated", which wrongly suggested such a pod would error.

**Per-language:** `<lang>-container-names`, only read when `--enable-multi-instrumentation` is true
(`podmutator.go:340-352`, `setLanguageSpecificContainers` at `podmutator.go:148`). When
multi-instrumentation is enabled, per-language names are **appended** to whatever
`container-names` already set (`setContainersFromAnnotation` uses `append`,
`helper.go:171`). `docs/auto-instrumentation/multi-instrumentation.md` states that
`container-names` "is not used for this feature", but the code path in `Mutate` runs
`setCommonInstrumentedContainers` before the per-language step regardless of the
multi-instrumentation flag, so both are in fact combined.

**Semantics when absent:** `ensureContainer` (`internal/instrumentation/sdk.go:265`):

```go
func ensureContainer(inst *instrumentationWithContainers, pod corev1.Pod) {
	if len(inst.Containers) == 0 {
		inst.Containers = []string{pod.Spec.Containers[0].Name}
	}
}
```

So the default is **the first regular container only** (`pod.Spec.Containers[0]`), never "all
containers" and never an init container.

**Init containers are addressable.** Names in the annotation are matched against
`pod.Spec.Containers` *and* `pod.Spec.InitContainers` (`findContainerByName`, `sdk.go:327`).
`containersToInstrument` (`sdk.go:273`) orders targeted init containers first (in pod-spec order),
then regular containers, and `insertInitContainer` (`sdk.go:345`) inserts the agent init container
*before* the target init container rather than appending. Supported for Java, Python, Node.js, .NET
and sdk-only; not for Go, Apache HTTPD, Nginx (`docs/auto-instrumentation/init-containers.md`).

**Validation:** the annotation value must match `^[a-zA-Z0-9-,]+$` (`isValidContainersAnnotation`,
`helper.go:145`). Dots and underscores are rejected, and an invalid value makes `Mutate` return an
error (pod created uninstrumented). Duplicate names within one language's list are rejected
(`findDuplicatedContainers`, `helper.go:70`).

**Multi-instrumentation consistency rules** (`areInstrumentedContainersCorrect`,
`podmutator.go:79`), only applied when `--enable-multi-instrumentation` is on: it is an error to
mix instrumentations that have explicit container names with ones that don't, or to have more than
one instrumentation without container names. When the check fails, injection is **silently
skipped** (`return pod, nil` at `podmutator.go:348-351`), not errored.

---

## B. What is injected, per language

Shared constants (`internal/instrumentation/sdk.go:34-38`):
`volumeName = initContainerName = sideCarName = "opentelemetry-auto-instrumentation"`.

### B.5–B.7 — Init container, volume, volume mount

| | Java | Node.js | Python | .NET |
|---|---|---|---|---|
| Init container name | `opentelemetry-auto-instrumentation-java` | `opentelemetry-auto-instrumentation-nodejs` | `opentelemetry-auto-instrumentation-python` | `opentelemetry-auto-instrumentation-dotnet` |
| Image field | `spec.java.image` | `spec.nodejs.image` | `spec.python.image` | `spec.dotnet.image` |
| Default image (operator flag) | `ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-java:<versions.txt>` | `…/autoinstrumentation-nodejs:<v>` | `…/autoinstrumentation-python:<v>` | `…/autoinstrumentation-dotnet:<v>` |
| `command` | `["cp", "/javaagent.jar", "/otel-auto-instrumentation-java/javaagent.jar"]` | `["cp", "-r", "/autoinstrumentation/.", "/otel-auto-instrumentation-nodejs"]` | `["cp", "-r", "<src>", "/otel-auto-instrumentation-python"]` where `<src>` = `/autoinstrumentation/.` (glibc) or `/autoinstrumentation-musl/.` (musl) | `["cp", "-r", "/autoinstrumentation/.", "/otel-auto-instrumentation-dotnet"]` |
| `args` | none | none | none | none |
| Init-container mountPath | `/otel-auto-instrumentation-java` | `/otel-auto-instrumentation-nodejs` | `/otel-auto-instrumentation-python` | `/otel-auto-instrumentation-dotnet` |
| Volume name | `opentelemetry-auto-instrumentation-java` | `…-nodejs` | `…-python` | `…-dotnet` |
| Volume type | `emptyDir` with `sizeLimit`, **or** `ephemeral.volumeClaimTemplate` if `spec.<lang>.volumeClaimTemplate` is non-zero | same | same | same |
| Default size limit | `200Mi` (`helper.go:21`, `defaultSize`) | `200Mi` | `200Mi` | `200Mi` |
| **App-container mountPath** | **`/otel-auto-instrumentation-java-<containerName>`** (per container!) | `/otel-auto-instrumentation-nodejs` | `/otel-auto-instrumentation-python` | `/otel-auto-instrumentation-dotnet` |

Sources: `javaagent.go:14-79`, `nodejs.go:12-59`, `python.go:14-101`, `dotnet.go:15-97`;
volume construction `helper.go:116` (`instrVolume`), default images
`internal/config/config.go:206-212`, defaulting onto the CR
`internal/webhook/instrumentation_webhook.go:67-172`.

Notable asymmetry: **Java is the only language whose app-container mount path is per-container**
(`fmt.Sprintf("%s-%s", javaInstrMountPath, container.Name)`, `javaagent.go:34`). The init container
still writes to the plain `/otel-auto-instrumentation-java` path — same volume, different mount
point. The other three mount at a fixed path shared by all instrumented containers.

Java also injects one extra init container per entry in `spec.java.extensions`
(`javaagent.go:65-78`): name `opentelemetry-auto-instrumentation-extension-<i>`, image
`extensions[i].image`, command `["cp", "-r", "<dir>/.", "/otel-auto-instrumentation-java/extensions"]`.

The init container (and the extension containers) is added **once per pod**, guarded by
`isInitContainerMissing` (`helper.go:24`), keyed on `containers[0].Name` for the insert position.

### B.8 — Environment variables set on the application container

#### Java (`javaagent.go:98`, applied via `appendOrReplace`, `sdk.go:845`)

| Env var | Value construction |
|---|---|
| `JAVA_TOOL_OPTIONS` | `<existing value> + " -javaagent:/otel-auto-instrumentation-java-<containerName>/javaagent.jar"`, plus `" -Dotel.javaagent.extensions=/otel-auto-instrumentation-java-<containerName>/extensions"` when `spec.java.extensions` is non-empty. **Appends** — note the value begins with a space. If unset, the value is just the agent argument (with a leading space). |

If the container's `JAVA_TOOL_OPTIONS` uses `valueFrom`, `validateContainerEnv` (`sdk.go:869`)
makes `injectJavaagentToContainer` fail and **the whole container is skipped** (logged, no volume
mount, no env). If it uses `valueFrom` and somehow got past that, `getDefaultJavaEnvVars` returns
an empty list (`javaagent.go:116-118`).

#### Node.js (`nodejs.go:76`, applied via `appendOrReplace`)

| Env var | Value construction |
|---|---|
| `NODE_OPTIONS` | `<existing value> + " --require /otel-auto-instrumentation-nodejs/autoinstrumentation.js"`. **Appends**; leading space. Skipped if `valueFrom`. |
| `OTEL_METRICS_EXPORTER` | `"otlp"` — only if not already set. Comment in source: the Node.js SDK only initialises metrics when a reader is configured. |

`NODE_OPTIONS` with `valueFrom` fails `validateContainerEnv` → container skipped.

#### Python (`python.go:43` for `PYTHONPATH`, `python.go:117` for the rest)

| Env var | Value construction |
|---|---|
| `PYTHONPATH` | If unset: `"/otel-auto-instrumentation-python/opentelemetry/instrumentation/auto_instrumentation:/otel-auto-instrumentation-python"`. If set: `"<prefix>:<existing>:<suffix>"` — i.e. **wrapped**, prefix prepended *and* suffix appended. Skipped (container skipped) if `valueFrom`. |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `"http/protobuf"` — `appendIfNotSet` |
| `OTEL_TRACES_EXPORTER` | `"otlp"` — `appendIfNotSet` |
| `OTEL_METRICS_EXPORTER` | `"otlp"` — `appendIfNotSet` |
| `OTEL_LOGS_EXPORTER` | `"otlp"` — `appendIfNotSet` |

Note Python's defaults are applied with `appendIfNotSet` (`sdk.go:415`), unlike Java/Node.js which
use `appendOrReplace`.

Python is also the only language forcing `http/protobuf`, which means an `spec.exporter.endpoint`
pointing at port 4317 will not work for Python (documented in
`docs/rfcs/instrumentation-v1beta1.md` §2 and in `docs/auto-instrumentation/README.md`).

#### .NET (`dotnet.go:113`, via `setDotNetEnvVar`, `dotnet.go:140`)

All paths are under the fixed mount `/otel-auto-instrumentation-dotnet`.

| Env var | Value | Merge behaviour |
|---|---|---|
| `CORECLR_ENABLE_PROFILING` | `"1"` | set only if absent; existing value wins |
| `CORECLR_PROFILER` | `"{918728DD-259F-4A6A-AC2B-B85E1B658318}"` | set only if absent |
| `CORECLR_PROFILER_PATH` | glibc: `/otel-auto-instrumentation-dotnet/linux/OpenTelemetry.AutoInstrumentation.Native.so`; musl: `/otel-auto-instrumentation-dotnet/linux-musl/OpenTelemetry.AutoInstrumentation.Native.so` (selected by the `otel-dotnet-auto-runtime` annotation) | set only if absent |
| `DOTNET_STARTUP_HOOKS` | `/otel-auto-instrumentation-dotnet/net/OpenTelemetry.AutoInstrumentation.StartupHook.dll` | **concatenated**: `"<existing>:<value>"` |
| `DOTNET_ADDITIONAL_DEPS` | `/otel-auto-instrumentation-dotnet/AdditionalDeps` | **concatenated** with `:` |
| `DOTNET_SHARED_STORE` | `/otel-auto-instrumentation-dotnet/store` | **concatenated** with `:` |
| `OTEL_DOTNET_AUTO_HOME` | `/otel-auto-instrumentation-dotnet` | set only if absent |

Pre-conditions (`dotnet.go:42-63`): the container is skipped if `DOTNET_STARTUP_HOOKS`,
`DOTNET_ADDITIONAL_DEPS` or `DOTNET_SHARED_STORE` uses `valueFrom`; or if `OTEL_DOTNET_AUTO_HOME`
is already set on the container *or* in `spec.dotnet.env`; or if the runtime annotation is not one
of `""`, `linux-x64`, `linux-musl-x64`.

If the runtime annotation value is invalid, `injectDotNetSDKToContainer` errors out first, so
`CORECLR_PROFILER_PATH` is never left empty in practice.

#### Plus, for all four languages

`spec.<lang>.env` (`appendIfNotSet`, applied inside `inject<Lang>SDKToContainer`), then the common
set from section C.13.

### B.9 — Init container securityContext and resources

**securityContext** (`resolveInitContainerSecurityContext`, `sdk.go:314`):

```go
func resolveInitContainerSecurityContext(specSecurityContext, containerSecurityContext *corev1.SecurityContext) *corev1.SecurityContext {
	if specSecurityContext != nil {
		return specSecurityContext
	}
	return containerSecurityContext
}
```

So: `spec.initContainerSecurityContext` if set, otherwise **the init container inherits the
securityContext of the first instrumented application container**. There is no hardcoded default
(no `runAsNonRoot`, no dropped capabilities). If the app container has no securityContext, the init
container gets none either.

**Resources**: the injector copies `spec.<lang>.resources` (Java: JSON tag `resources`; the others:
`resourceRequirements`) verbatim onto the init container. Those fields are populated by the
`Instrumentation` **defaulting webhook**, not by the pod webhook
(`internal/webhook/instrumentation_webhook.go:67-172`):

| Language | Limits | Requests |
|---|---|---|
| Java | `cpu: 500m`, `memory: 256Mi` | `cpu: 50m`, `memory: 64Mi` |
| Node.js | `cpu: 500m`, `memory: 256Mi` | `cpu: 50m`, `memory: 128Mi` |
| Python | `cpu: 500m`, `memory: 256Mi` | `cpu: 50m`, `memory: 64Mi` |
| .NET | `cpu: 500m`, `memory: 256Mi` | `cpu: 50m`, `memory: 128Mi` |
| Go | `cpu: 500m`, `memory: 256Mi` | `cpu: 50m`, `memory: 64Mi` |
| Apache HTTPD / Nginx | `cpu: 500m`, `memory: 256Mi` | `cpu: 1m`, `memory: 128Mi` |

`imagePullPolicy` on the init container comes from `spec.imagePullPolicy` (empty by default, so
Kubernetes' own default applies).

The Apache HTTPD *clone* init container is the exception: it copies `securityContext` and
`imagePullPolicy` from the application container, not from the spec (`apachehttpd.go:86-87`). The
Nginx clone container uses `resolveInitContainerSecurityContext` (`nginx.go:84`).

---

## C. Config translation (spec fields → env vars)

### C.10 — Complete `v1alpha1` `InstrumentationSpec` field list

Source: `apis/v1alpha1/instrumentation_types.go`. API docs: `docs/api/instrumentations.md`.

#### Top level

| Field (YAML) | Go field | Produces |
|---|---|---|
| `exporter.endpoint` | `Exporter.Endpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` (`appendIfNotSet`), `exporter.go:18-23` |
| `exporter.tls.ca_file` | `TLS.CA` | `OTEL_EXPORTER_OTLP_CERTIFICATE` = `<configMapMountPath or secretMountPath>/<ca_file>`, or the literal value if it is an absolute path |
| `exporter.tls.cert_file` | `TLS.Cert` | `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE` = `<secretMountPath>/<cert_file>`, or the literal absolute path |
| `exporter.tls.key_file` | `TLS.Key` | `OTEL_EXPORTER_OTLP_CLIENT_KEY` = `<secretMountPath>/<key_file>`, or the literal absolute path |
| `exporter.tls.secretName` | `TLS.SecretName` | no env var; adds a `secret` volume `otel-auto-secret-<name>` (truncated to 63 chars) mounted read-only at `/otel-auto-instrumentation-secret-<name>`. Also validated for existence in the **pod's** namespace before injection (`podmutator.go:424`) |
| `exporter.tls.configMapName` | `TLS.ConfigMapName` | no env var; adds a `configMap` volume `otel-auto-configmap-<name>` mounted read-only at `/otel-auto-instrumentation-configmap-<name>`; existence validated |
| `resource.resourceAttributes` | `Resource.Attributes` | merged into `OTEL_RESOURCE_ATTRIBUTES`, **lowest** precedence (`sdk.go:690-694`) |
| `resource.addK8sUIDAttributes` | `Resource.AddK8sUIDAttributes` | adds `k8s.{pod,deployment,replicaset,statefulset,daemonset,job,cronjob}.uid` to `OTEL_RESOURCE_ATTRIBUTES`, plus the `OTEL_RESOURCE_ATTRIBUTES_POD_UID` downward-API env var |
| `propagators[]` | `Propagators` | `OTEL_PROPAGATORS`, comma-joined, only if not already set and the list is non-empty (`sdk.go:504-511`) |
| `sampler.type` | `Sampler.Type` | `OTEL_TRACES_SAMPLER` |
| `sampler.argument` | `Sampler.Argument` | `OTEL_TRACES_SAMPLER_ARG` (only emitted when `type` is also set) |
| `defaults.useLabelsForResourceAttributes` | `Defaults.UseLabelsForResourceAttributes` | no env var; enables reading `app.kubernetes.io/{instance,name,version}` labels for `service.name` / `service.version` |
| `env[]` | `Env` | appended verbatim to the instrumented container, `appendIfNotSet` (`sdk.go:395-400`) |
| `imagePullPolicy` | `ImagePullPolicy` | `imagePullPolicy` on the injected init containers / Go sidecar |
| `initContainerSecurityContext` | | `securityContext` on init containers (not the Go sidecar) |

Sampler gating detail (`sdk.go:513-529`): the sampler block is emitted only when
`OTEL_TRACES_SAMPLER` **and** `OTEL_TRACES_SAMPLER_ARG` are both absent from the container.
Setting only `OTEL_TRACES_SAMPLER_ARG` yourself suppresses both.

#### Per-language blocks

`java`, `nodejs`, `python`, `dotnet`, `go`, `apacheHttpd`, `nginx` share:

| Field | Produces |
|---|---|
| `image` | init container / sidecar image |
| `volumeClaimTemplate` | `ephemeral` volume instead of `emptyDir` |
| `volumeLimitSize` (**deprecated**, Go field `VolumeSizeLimit`) | `emptyDir.sizeLimit`; defaults to `200Mi`. Mutually exclusive with `volumeClaimTemplate` (webhook rejects both) |
| `env[]` | appended to the instrumented container (`appendIfNotSet`), **before** the language default env vars are computed — so e.g. a `JAVA_TOOL_OPTIONS` set here will be appended to, not replaced |
| `resources` (Java) / `resourceRequirements` (all others) | init container / sidecar resources. The JSON tag inconsistency is real and is called out in the v1beta1 RFC |

Language-specific extras:

| Field | Produces |
|---|---|
| `java.extensions[].image` / `.dir` | extra init containers + `-Dotel.javaagent.extensions=<mount>/extensions` appended to `JAVA_TOOL_OPTIONS` |
| `go.securityContext` | securityContext of the Go sidecar; when unset, hardcoded `{RunAsUser: 0, Privileged: true}` |
| `apacheHttpd.version` | `2.4` (default) or `2.2` — selects `libmod_apache_otel.so` vs `libmod_apache_otel22.so` |
| `apacheHttpd.configPath` | default `/usr/local/apache2/conf`; where the config is cloned from/to |
| `apacheHttpd.attrs[]` | key/value lines in the generated `opentemetry_agent.conf`, overriding the operator defaults |
| `nginx.configFile` | default `/etc/nginx/nginx.conf` |
| `nginx.attrs[]` | key/value lines (with `;` terminators) in the generated Nginx module config |

`status.upgradeBlockedVersions` is the only status field.

### C.11 — Documented 4-layer env var precedence

Quoted verbatim from `apis/v1alpha1/instrumentation_types.go:35-38` (and repeated on every
per-language `Env` field):

> Env defines common env vars. There are four layers for env vars' definitions and
> the precedence order is: `original container env vars` > `language specific env vars` > `common env vars` > `instrument spec configs' vars`.
> If the former var had been defined, then the other vars would be ignored.

**Flagged caveat (factual):** this rule holds for anything applied with `appendIfNotSet`
(`sdk.go:828`), which covers `spec.env`, `spec.<lang>.env`, `OTEL_EXPORTER_OTLP_ENDPOINT`,
`OTEL_PROPAGATORS`, the sampler vars, and Python's exporter defaults. It does **not** hold for the
agent-activation variables, which are applied with `appendOrReplace` (`sdk.go:845`) or explicit
concatenation and therefore modify a user-supplied value rather than defer to it:

- `JAVA_TOOL_OPTIONS`, `NODE_OPTIONS` — the agent argument is appended to the existing value
- `PYTHONPATH` — the existing value is wrapped between the operator's prefix and suffix
- `DOTNET_STARTUP_HOOKS`, `DOTNET_ADDITIONAL_DEPS`, `DOTNET_SHARED_STORE` — colon-concatenated
- `OTEL_RESOURCE_ATTRIBUTES` — the computed attribute string is appended to the existing value
  (`sdk.go:490-502`)

The escape hatch upstream honours is `valueFrom`: if the user set the variable via `valueFrom`, the
operator refuses to touch it, and for Java/Node.js/Python/.NET that means it **skips the whole
container**.

### C.12 — Automatically injected resource attributes and service name

`createResourceMap` (`sdk.go:672`) builds the attribute map, `injectCommonSDKConfig`
(`sdk.go:432`) serialises it. Precedence within `OTEL_RESOURCE_ATTRIBUTES`, lowest to highest:

1. `spec.resource.resourceAttributes` from the CR
2. K8s-derived attributes
3. pod annotations prefixed `resource.opentelemetry.io/`
4. `service.namespace` from `chooseServiceNamespace`

Anything already present as a key in a pre-existing `OTEL_RESOURCE_ATTRIBUTES` on the container is
never overwritten (`existingRes` map, `sdk.go:674-685`).

K8s-derived attributes (`sdk.go:699-708` plus `addParentResourceLabels`, `sdk.go:733`):

| Attribute | Source |
|---|---|
| `k8s.namespace.name` | namespace object name |
| `k8s.container.name` | the instrumented container's name |
| `k8s.pod.name` | `$(OTEL_RESOURCE_ATTRIBUTES_POD_NAME)` — always replaced with the downward-API reference |
| `k8s.pod.uid` | `pod.UID`, or `$(OTEL_RESOURCE_ATTRIBUTES_POD_UID)` when `addK8sUIDAttributes` and the UID is empty |
| `k8s.node.name` | `pod.Spec.NodeName`, or `$(OTEL_RESOURCE_ATTRIBUTES_NODE_NAME)` when empty (which it always is at admission time) |
| `service.instance.id` | `<namespace>.$(OTEL_RESOURCE_ATTRIBUTES_POD_NAME).<containerName>` |
| `k8s.replicaset.name` / `k8s.deployment.name` / `k8s.statefulset.name` / `k8s.daemonset.name` / `k8s.job.name` / `k8s.cronjob.name` | walked from `ownerReferences`; **ReplicaSet → Deployment and Job → CronJob need an extra `Get`**, served by the manager's informer cache rather than the API server. Wrapped in a retry backoff that fires on `NotFound` only (10 ms base, factor 1.5, 20 steps, 2 s cap) — itself evidence the read is cached, since a `NotFound` from a direct read would be authoritative and worth no retry |
| `…uid` variants of the above | only when `spec.resource.addK8sUIDAttributes: true` |
| `service.version` | `chooseServiceVersion` (see below) |
| `service.namespace` | `resource.opentelemetry.io/service.namespace` annotation, else the namespace name |

The final string is produced by `resourceMapToStr` (`sdk.go:801`): `k=v` pairs joined by `,`, keys
**sorted alphabetically**.

**`service.name` derivation** — `chooseServiceName`, `sdk.go:545`, first non-empty wins:

1. pod annotation `resource.opentelemetry.io/service.name`
2. if `defaults.useLabelsForResourceAttributes`: pod label `app.kubernetes.io/instance`, then
   `app.kubernetes.io/name`
3. `k8s.deployment.name`
4. `k8s.replicaset.name`
5. `k8s.statefulset.name`
6. `k8s.daemonset.name`
7. `k8s.cronjob.name`
8. `k8s.job.name`
9. `k8s.pod.name` (which is the literal string `$(OTEL_RESOURCE_ATTRIBUTES_POD_NAME)`)
10. the container's name

It is written to `OTEL_SERVICE_NAME` only if that variable is not already on the container
(`sdk.go:435-441`). This order matches `docs/auto-instrumentation/resource-attributes.md`; the
doc's final `k8s.container.name` entry corresponds to `container.Name` in the code.

**`service.version` derivation** — `chooseServiceVersion`, `sdk.go:593`:

1. pod annotation `resource.opentelemetry.io/service.version`
2. if `useLabelsForResourceAttributes`: pod label `app.kubernetes.io/version`
3. parsed from the container image reference (`parseServiceVersionFromImage`, `sdk.go:620`):
   `tag@digest` if both, else `digest`, else `tag`, else nothing

It is only computed when `OTEL_RESOURCE_ATTRIBUTES` is absent or does not already contain
`service.version` (`sdk.go:470-476`).

**`service.instance.id`** — `createServiceInstanceId`, `sdk.go:652`: the
`resource.opentelemetry.io/service.instance.id` annotation, else
`<namespace>.<podName>.<containerName>`. Labels are deliberately not consulted here (uniqueness
requirement).

### C.13 — Injected unconditionally, regardless of language

`injectCommonEnvVar` (`sdk.go:388`) — **prepended** to the front of the container's env list, so
they precede everything else:

| Env var | Source |
|---|---|
| `OTEL_NODE_IP` | `fieldRef: status.hostIP` |
| `OTEL_POD_IP` | `fieldRef: status.podIP` |

(Because each is prepended in turn, the resulting order at the head of the list is `OTEL_NODE_IP`,
then `OTEL_POD_IP`.)

`injectCommonSDKConfig` (`sdk.go:432`) — appended:

| Env var | Condition |
|---|---|
| `OTEL_SERVICE_NAME` | if not already set |
| `OTEL_EXPORTER_OTLP_ENDPOINT` (+ TLS vars) | if `spec.exporter.endpoint` set |
| `OTEL_RESOURCE_ATTRIBUTES_POD_NAME` (`fieldRef: metadata.name`) | **always**, unconditionally |
| `OTEL_RESOURCE_ATTRIBUTES_POD_UID` (`fieldRef: metadata.uid`) | only if `addK8sUIDAttributes` and `k8s.pod.uid` is empty |
| `OTEL_RESOURCE_ATTRIBUTES_NODE_NAME` (`fieldRef: spec.nodeName`) | if `k8s.node.name` is empty — which it always is at admission time, so effectively always |
| `OTEL_PROPAGATORS` | if not set and `spec.propagators` non-empty |
| `OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG` | if neither is set and `spec.sampler.type` non-empty |
| `OTEL_RESOURCE_ATTRIBUTES` | always; appended to an existing value with a `,` separator |

**Env-var ordering is load-bearing.** The last thing `injectCommonSDKConfig` does is move
`OTEL_RESOURCE_ATTRIBUTES` to the **end** of the list (`moveEnvToListEnd`, `sdk.go:536-538`),
because its value contains `$(OTEL_RESOURCE_ATTRIBUTES_POD_NAME)` / `$(…_NODE_NAME)` /
`$(…_POD_UID)` and Kubernetes only expands `$(VAR)` against variables declared **earlier** in the
same container's `env` list. Getting this order wrong yields literal `$(VAR)` strings in the
telemetry. This is tracked upstream as issue #3022 and is one of the motivations for the v1beta1
declarative-config work.

The sdk-only mode (`inject-sdk`) injects exactly this common set and nothing else — no init
container, no volume, no agent (`injectSdk`, `sdk.go:253`).

---

## D. Architecturally different languages

### D.14 — Go

Go is an **eBPF sidecar**, not an init container (`internal/instrumentation/golang.go:23`,
`injectGoSDK`). The operator appends a regular container named `opentelemetry-auto-instrumentation`
to `pod.Spec.Containers`, using `spec.go.image` (default
`ghcr.io/open-telemetry/opentelemetry-go-instrumentation/autoinstrumentation-go:<v>`), forces
`pod.Spec.ShareProcessNamespace = true`, and mounts a **hostPath** volume `kernel-debug` →
`/sys/kernel/debug`. The sidecar's securityContext defaults to `{RunAsUser: 0, Privileged: true}`
unless `spec.go.securityContext` overrides it. All SDK config env vars (including
`OTEL_RESOURCE_ATTRIBUTES`, computed from the *application* container) are placed on the **sidecar**,
not on the app container — the app container is left completely untouched. `OTEL_GO_AUTO_TARGET_EXE`
is mandatory: it comes from the `instrumentation.opentelemetry.io/otel-go-auto-target-exe`
annotation (highest precedence) or from `spec.go.env`; if it ends up unset, the operator **reverts
the pod to its original state** and skips injection (`sdk.go:200-205`). Injection is also refused if
`shareProcessNamespace` is explicitly `false`, if more than one container name is targeted, or if
the target is an init container. Requires privileged pod security — a `restricted` PodSecurity
namespace will reject the resulting pod.

### D.15 — Apache HTTPD and Nginx

Both are **config-file based**, not env-var based, and both use a two-init-container "clone and
patch" scheme (`internal/instrumentation/apachehttpd.go`, `internal/instrumentation/nginx.go`;
the mechanism is documented in the block comments at the top of each file).

Step 1: an init container named `otel-agent-source-container-clone` is created **from the
application container's own image**, inheriting its `env`, `envFrom`, `volumeMounts`,
`securityContext` and `imagePullPolicy`, whose only job is to copy the existing server config out of
the app image into a shared `emptyDir` (`otel-apache-conf-dir` / `otel-nginx-conf-dir`). Step 2: a
second init container (`otel-agent-attach-apache` / `otel-agent-attach-nginx`) runs the
instrumentation image with `/bin/sh -c <embedded script> -- <configDir|configFile>`; the script
patches the cloned config to load the OTel webserver module and writes a generated
`opentemetry_agent.conf` whose content is passed in via the `OTEL_APACHE_AGENT_CONF` /
`OTEL_NGINX_AGENT_CONF` env var, then copies the module into a second `emptyDir`
(`otel-apache-agent` / `otel-nginx-agent`). Step 3: the operator **removes** any of the application
container's existing volumeMounts whose mountPath contains the config dir (so a ConfigMap-supplied
config would be dropped) and mounts the two shared volumes in their place. Service name, endpoint
and attributes are rendered into the config file text, not into env vars; the endpoint defaults to
`http://localhost:4317/` and only gRPC is supported by the underlying module. Nginx additionally
appends `<agentDir>/sdk_lib/lib` to `LD_LIBRARY_PATH` on the app container. `service.instance.id` is
written as the placeholder `<<SID-PLACEHOLDER>>` and substituted at container start from
`APACHE_SERVICE_INSTANCE_ID` / `OTEL_NGINX_SERVICE_INSTANCE_ID` (downward API `metadata.name`).
Neither supports init containers as targets.

---

## E. Schema evolution (v1beta1)

Source: `docs/rfcs/instrumentation-v1beta1.md`. **Status: RFC only — not implemented.** No
`apis/v1beta1/instrumentation_types.go` exists on `main` at the commit examined (the `v1beta1`
package exists but only for the Collector types).

Proposed changes:

1. **`declarativeConfig` vs `envConfig`.** `spec.declarativeConfig` holds strongly-typed structs
   matching the [OTel declarative configuration schema](https://github.com/open-telemetry/opentelemetry-configuration);
   the operator serialises it to YAML, mounts it, and sets `OTEL_CONFIG_FILE`. `spec.envConfig`
   holds today's `exporter` / `sampler` / `propagators`. The two are **mutually exclusive** and the
   webhook rejects setting both. Languages without declarative-config support get injection
   **skipped** with a warning event, gated by a new `--instrumentation-declarative-config=<langs>`
   flag. In declarative mode the operator merges computed resource attributes directly into the
   YAML instead of emitting `OTEL_RESOURCE_ATTRIBUTES`, which sidesteps the `$(VAR)`-ordering
   problem (issue #3022).
2. **Explicit exporter `protocol`** field (`grpc` / `http/protobuf` / `http/json`) → emits
   `OTEL_EXPORTER_OTLP_PROTOCOL`.
3. **Per-language field normalisation**: `volumeLimitSize` removed; `resourceRequirements` renamed
   to `resources` everywhere; shared `CommonLanguageSpec` base.
4. **`Resource` and `Defaults` consolidated** into a single top-level `spec.resource` with
   `attributes`, `k8sMetadata{enabled,includeUIDs}` and `serviceMetadata{enabled}`. This field
   applies in both config modes because it controls operator-level injection, not SDK config.
5. **Injection control moves from annotations to labels**, same keys, same value semantics. Phase 1
   supports both with **labels taking precedence** over annotations; Phase 2 drops annotations and
   adds an `objectSelector` to the MutatingWebhookConfiguration so only opted-in pods reach the
   webhook.
6. **No default instrumentation image**: per-language `image` becomes required; the operator stops
   reading `versions.txt` for defaults, decoupling operator releases from SDK releases.

**Is v1alpha1 expected to remain served?** The RFC's Migration Strategy says: a conversion webhook
between v1alpha1 and v1beta1; "Dual-version support: Serve both v1alpha1 and v1beta1 simultaneously
with v1beta1 as the storage version"; and "v1alpha1 is deprecated when v1beta1 ships". The
annotation deprecation path references a "Phase 2 — v1alpha1 removal" but gives **no date or release
target**. So: v1alpha1 stays served through the v1beta1 transition, with removal planned but
unscheduled. See also `docs/rfcs/crd-version-graduation-strategies.md`.

---

## Questions not answered definitively

- **Nothing in sections A–D was left unresolved**; all answers above are read directly from source.
- **E (v1beta1)** is a proposal document only. Any statement about v1beta1 is a statement about the
  RFC, not about shipped behaviour. The removal date for v1alpha1 is explicitly not specified
  upstream.
- The `container-names` vs `<lang>-container-names` interaction under
  `--enable-multi-instrumentation` is **contradictory between code and docs**: the doc says
  `container-names` is not used, the code appends both. I have described the code behaviour and
  flagged the doc mismatch rather than picking one.
- `chooseServiceName`'s ordering of `k8s.cronjob.name` / `k8s.job.name` was verified against the
  code (cronjob before job); the rendered website page and the in-repo doc agree with the code.

## Citation index

| Topic | File |
|---|---|
| Annotation keys, pod/ns precedence | `internal/instrumentation/annotation.go` |
| CR selection, kill switches, container annotations | `internal/instrumentation/podmutator.go` |
| Volume/emptyDir, container validation, idempotency guard | `internal/instrumentation/helper.go` |
| Common env, resource map, service name, env ordering | `internal/instrumentation/sdk.go` |
| Per-language injection | `internal/instrumentation/{javaagent,nodejs,python,dotnet,golang,apachehttpd,nginx}.go` |
| Exporter + TLS | `internal/instrumentation/exporter.go` |
| CRD types | `apis/v1alpha1/instrumentation_types.go` |
| CR defaulting (images, resources) + validation | `internal/webhook/instrumentation_webhook.go` |
| Pod webhook wiring | `internal/webhook/podmutation/webhookhandler.go` |
| Operator flags and defaults | `internal/config/{config,cli}.go` |
| Env var name constants | `pkg/constants/env.go` |
| Default image versions | `versions.txt` |
| Docs | `docs/auto-instrumentation/{README,multi-container,multi-instrumentation,init-containers,resource-attributes,custom-images}.md`, `docs/api/instrumentations.md` |
| v1beta1 RFC | `docs/rfcs/instrumentation-v1beta1.md` |
| Website | <https://opentelemetry.io/docs/platforms/kubernetes/operator/automatic/> |
