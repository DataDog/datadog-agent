// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package otelinstrumentation

import (
	"sort"
	"strings"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Passthrough mode injects the community OpenTelemetry SDK and passes the custom
// resource's configuration through unchanged, which means reproducing upstream's pod
// mutation rather than mapping it onto Datadog's. The contract is upstream's and is not
// negotiable: the SDK image only contains its payload under /autoinstrumentation, the
// init container has to copy it to /otel-auto-instrumentation-<lang>, and the runtime is
// configured by environment variables pointing there.
//
// This produces the mutation; it does not apply it. Building concrete corev1 objects and
// leaving the writes to the caller keeps upstream's contract in one testable place.
//
// # What this does not support
//
// Each entry below is a deliberate gap, not an oversight. The ones a custom resource can
// ask for are logged and counted (otel_instrumentation_passthrough_unsupported), so a
// surprised user can find out why a field had no effect.
//
//   - **musl-based images.** The otel-python-platform and otel-dotnet-auto-runtime pod
//     annotations are ignored: the copy source and the .NET profiler path are always the
//     glibc ones. A musl workload gets an init container that copies the wrong payload.
//   - **spec.<lang>.volumeClaimTemplate.** The payload always lands in an emptyDir.
//   - **spec.java.extensions.** Upstream adds one init container per extension and a
//     -Dotel.javaagent.extensions option; neither is produced here.
//   - **Kubernetes-derived resource attributes.** k8s.pod.name, k8s.pod.uid,
//     k8s.node.name, service.instance.id and the owner-derived names
//     (k8s.deployment.name and friends) are not emitted, and neither are the downward-API
//     variables they need. Upstream builds them with $(VAR) references, which only expand
//     against variables declared earlier in the same container, so emitting them means
//     owning that ordering; and the owner-derived ones need API reads this package does
//     not do. Only k8s.namespace.name and k8s.container.name, which are free of both
//     problems, are emitted. OTEL_NODE_IP and OTEL_POD_IP are the exception: they are
//     emitted, first in the list, because an exporter endpoint pointing at the node's
//     agent has no other way to name it.
//   - **spec.resource.addK8sUIDAttributes.** Follows from the above.
//   - **Owner-derived service names.** OTEL_SERVICE_NAME falls back to the container name
//     where upstream would have used the Deployment or StatefulSet name, for the same
//     reason. A workload with no service.name annotation or label is therefore named
//     after its container.
//   - **Exporter TLS material.** spec.exporter's certificate and key fields are not
//     emitted, only the endpoint.
//   - **Restricted namespaces.** The init container carries no security context, so a
//     namespace enforcing the restricted Pod Security Standard rejects the pod. Swap mode
//     resolves one through libraryinjection; passthrough deliberately does not reuse that
//     machinery yet.
//   - **CSI and image volumes.** Upstream knows only emptyDir plus an init container, and
//     so does this.
//   - **Languages upstream supports and Datadog does not** (go, apache-httpd, nginx) and
//     the env-vars-only inject-sdk mode: out of scope, see MappableLanguages.
const (
	// passthroughResourcePrefix is completed by the language name to give upstream's
	// volume and init container names.
	passthroughResourcePrefix = "opentelemetry-auto-instrumentation-"

	// passthroughMountPrefix is completed by the language name to give upstream's mount
	// path. Java appends the container name to it, the other languages do not.
	passthroughMountPrefix = "/otel-auto-instrumentation-"

	// dotnetProfilerID is upstream's fixed CORECLR_PROFILER GUID.
	dotnetProfilerID = "{918728DD-259F-4A6A-AC2B-B85E1B658318}"

	// OpenTelemetry variables this mode emits beyond the per-language runtime wiring.
	envOTelResourceAttributes = "OTEL_RESOURCE_ATTRIBUTES"
	envOTelExporterEndpoint   = "OTEL_EXPORTER_OTLP_ENDPOINT"

	// Resource attributes that cost neither an API read nor a $(VAR) reference.
	attrK8sNamespaceName = "k8s.namespace.name"
	attrK8sContainerName = "k8s.container.name"
	attrServiceNamespace = "service.namespace"
)

// defaultVolumeSizeLimit is upstream's defaultSize for the instrumentation volume.
var defaultVolumeSizeLimit = resource.MustParse("200Mi")

// EnvMerge says how a variable combines with a value the container already declares.
// Upstream uses all three, and which one applies to which variable is part of its
// contract: appending to PYTHONPATH instead of wrapping it, for instance, leaves the
// SDK's sitecustomize.py unreachable.
type EnvMerge int

const (
	// EnvSetIfAbsent leaves an existing value alone. Upstream's appendIfNotSet.
	EnvSetIfAbsent EnvMerge = iota

	// EnvAppend produces "<existing><separator><value>". Note that upstream's values
	// for JAVA_TOOL_OPTIONS and NODE_OPTIONS start with a space and use no separator.
	EnvAppend

	// EnvWrap produces "<value><separator><existing><separator><suffix>", which is what
	// PYTHONPATH needs: the SDK's auto_instrumentation directory has to come first so
	// its sitecustomize.py wins, and its package directory last.
	EnvWrap
)

// EnvVar is one environment variable to apply to a container, with the merge behaviour
// upstream applies to it.
type EnvVar struct {
	Name  string
	Value string
	// ValueFrom is set instead of Value for the downward-API variables, and only
	// alongside EnvSetIfAbsent — a reference cannot be merged into anything.
	ValueFrom *corev1.EnvVarSource
	Suffix    string
	Separator string
	Merge     EnvMerge
}

// Apply returns the value to write for a container, given what it already declares.
func (e EnvVar) Apply(existing string) string {
	switch e.Merge {
	case EnvAppend:
		if existing == "" {
			return e.Value
		}
		return existing + e.Separator + e.Value
	case EnvWrap:
		if existing == "" {
			return e.Value + e.Separator + e.Suffix
		}
		return e.Value + e.Separator + existing + e.Separator + e.Suffix
	default:
		return e.Value
	}
}

// ContainerInjection is what one selected container gets.
type ContainerInjection struct {
	// Container is the container's name.
	Container string
	// Mount is where the SDK payload is mounted in that container. Java's path carries
	// the container name, so this genuinely differs per container.
	Mount corev1.VolumeMount
	// EnvVars are the variables to apply, in the order they must be written.
	EnvVars []EnvVar
}

// Injection is one language's passthrough mutation of a pod.
type Injection struct {
	Language Language
	// Volume holds the SDK payload, shared by the init container and the app containers.
	Volume corev1.Volume
	// InitContainer copies the payload out of the SDK image into Volume.
	InitContainer corev1.Container
	// Containers is one entry per selected container. Never empty.
	Containers []ContainerInjection
}

// Apply writes the injection into the pod. It is idempotent, so a webhook reinvocation
// replaces what the previous pass wrote instead of stacking a second copy of it — with
// one exception: an appended variable such as JAVA_TOOL_OPTIONS cannot tell our own
// previous value apart from the user's, so the caller must not apply the same injection
// twice to the same pod.
func (i Injection) Apply(pod *corev1.Pod) {
	patcher := libraryinjection.NewPodPatcher(pod, nil)
	patcher.AddVolume(i.Volume)
	patcher.AddInitContainer(i.InitContainer)

	for _, injection := range i.Containers {
		// Not PodPatcher.AddVolumeMountWithTarget: a container selection may name an
		// init container, which that method does not walk, and mounting nothing while
		// still writing the environment gives a container that fails at start.
		container := findContainer(pod, injection.Container)
		if container == nil {
			continue
		}
		addVolumeMount(container, injection.Mount)
		for _, env := range injection.EnvVars {
			applyEnvVar(container, env)
		}
	}
}

// addVolumeMount mounts the payload in a container, replacing an identical mount so that
// a second pass does not stack a duplicate.
func addVolumeMount(container *corev1.Container, mount corev1.VolumeMount) {
	for i, existing := range container.VolumeMounts {
		if existing.Name == mount.Name && existing.MountPath == mount.MountPath {
			container.VolumeMounts[i] = mount
			return
		}
	}
	container.VolumeMounts = append(container.VolumeMounts, mount)
}

// applyEnvVar writes one variable to one container, honouring its merge behaviour.
//
// New variables are appended rather than prepended, keeping the order this package
// produced. That order carries no meaning today because no value contains a $(VAR)
// reference; adding one would make it load-bearing, as it is upstream.
func applyEnvVar(container *corev1.Container, env EnvVar) {
	for i, existing := range container.Env {
		if existing.Name != env.Name {
			continue
		}
		if env.Merge == EnvSetIfAbsent || existing.ValueFrom != nil {
			return
		}
		container.Env[i].Value = env.Apply(existing.Value)
		return
	}
	if env.ValueFrom != nil {
		container.Env = append(container.Env, corev1.EnvVar{Name: env.Name, ValueFrom: env.ValueFrom})
		return
	}
	container.Env = append(container.Env, corev1.EnvVar{Name: env.Name, Value: env.Apply("")})
}

// languagePassthrough is upstream's per-language contract: how to copy the payload out
// of the SDK image, where to mount it, and how to point the runtime at it.
type languagePassthrough struct {
	// copyArgs is the init container's command, given the init container's mount path.
	copyArgs func(mountPath string) []string
	// mountPath returns an app container's mount path, given the language's base path.
	mountPath func(base, container string) string
	// env returns the runtime wiring for a container, given its mount path.
	env func(mountPath string) []EnvVar
}

// languagePassthroughs is the whole of upstream's injection contract for the four
// mappable languages, taken from its javaagent.go, nodejs.go, python.go and dotnet.go.
var languagePassthroughs = map[Language]languagePassthrough{
	Java: {
		// Java ships a single jar rather than a directory tree.
		copyArgs: func(mountPath string) []string {
			return []string{"cp", "/javaagent.jar", mountPath + "/javaagent.jar"}
		},
		// The only language whose app-container path is per container. Upstream does
		// this so that two containers of the same pod can carry different extensions.
		mountPath: func(base, container string) string { return base + "-" + container },
		env: func(mountPath string) []EnvVar {
			return []EnvVar{{
				Name:  "JAVA_TOOL_OPTIONS",
				Value: " -javaagent:" + mountPath + "/javaagent.jar",
				Merge: EnvAppend,
			}}
		},
	},
	NodeJS: {
		copyArgs:  copyTree,
		mountPath: sharedMountPath,
		env: func(mountPath string) []EnvVar {
			return []EnvVar{
				{
					Name:  "NODE_OPTIONS",
					Value: " --require " + mountPath + "/autoinstrumentation.js",
					Merge: EnvAppend,
				},
				// Upstream sets this because the Node.js SDK only starts metrics when a
				// reader is configured.
				{Name: "OTEL_METRICS_EXPORTER", Value: "otlp", Merge: EnvSetIfAbsent},
			}
		},
	},
	Python: {
		copyArgs:  copyTree,
		mountPath: sharedMountPath,
		env: func(mountPath string) []EnvVar {
			return []EnvVar{
				{
					Name:      "PYTHONPATH",
					Value:     mountPath + "/opentelemetry/instrumentation/auto_instrumentation",
					Suffix:    mountPath,
					Separator: ":",
					Merge:     EnvWrap,
				},
				// Python is the only language upstream forces onto http/protobuf, which
				// is why an exporter endpoint on port 4317 does not work for it.
				{Name: "OTEL_EXPORTER_OTLP_PROTOCOL", Value: "http/protobuf", Merge: EnvSetIfAbsent},
				{Name: "OTEL_TRACES_EXPORTER", Value: "otlp", Merge: EnvSetIfAbsent},
				{Name: "OTEL_METRICS_EXPORTER", Value: "otlp", Merge: EnvSetIfAbsent},
				{Name: "OTEL_LOGS_EXPORTER", Value: "otlp", Merge: EnvSetIfAbsent},
			}
		},
	},
	DotNet: {
		copyArgs:  copyTree,
		mountPath: sharedMountPath,
		env: func(mountPath string) []EnvVar {
			return []EnvVar{
				{Name: "CORECLR_ENABLE_PROFILING", Value: "1", Merge: EnvSetIfAbsent},
				{Name: "CORECLR_PROFILER", Value: dotnetProfilerID, Merge: EnvSetIfAbsent},
				{
					// The glibc path. Upstream selects linux-musl here from the
					// otel-dotnet-auto-runtime annotation, which this mode ignores.
					Name:  "CORECLR_PROFILER_PATH",
					Value: mountPath + "/linux/OpenTelemetry.AutoInstrumentation.Native.so",
					Merge: EnvSetIfAbsent,
				},
				{Name: "OTEL_DOTNET_AUTO_HOME", Value: mountPath, Merge: EnvSetIfAbsent},
				{
					Name:      "DOTNET_STARTUP_HOOKS",
					Value:     mountPath + "/net/OpenTelemetry.AutoInstrumentation.StartupHook.dll",
					Separator: ":",
					Merge:     EnvAppend,
				},
				{Name: "DOTNET_ADDITIONAL_DEPS", Value: mountPath + "/AdditionalDeps", Separator: ":", Merge: EnvAppend},
				{Name: "DOTNET_SHARED_STORE", Value: mountPath + "/store", Separator: ":", Merge: EnvAppend},
			}
		},
	},
}

// copyTree is the copy command every language but Java uses.
func copyTree(mountPath string) []string {
	return []string{"cp", "-r", "/autoinstrumentation/.", mountPath}
}

// sharedMountPath is the app-container path every language but Java uses: the same one
// the init container wrote to.
func sharedMountPath(base, _ string) string { return base }

// BuildInjections produces the passthrough mutations a resolution asks for, the
// counterpart of Translate: Translate serves the languages the mode discriminator sent to
// swap, this one serves those it sent to passthrough. A resolution can hold both.
func BuildInjections(result Result, pod *corev1.Pod) []Injection {
	if result.Outcome != OutcomeResolved {
		return nil
	}

	var injections []Injection
	for _, request := range result.Languages {
		if request.Mode != ModeOTel {
			continue
		}
		if injection, ok := BuildInjection(request, pod); ok {
			injections = append(injections, injection)
		}
	}
	return injections
}

// HasPassthroughInjection reports whether a pod already carries a passthrough injection,
// whether this webhook wrote it on an earlier reinvocation or the community Operator
// wrote it before us. Either way, injecting again would append a second -javaagent option
// to JAVA_TOOL_OPTIONS.
func HasPassthroughInjection(pod *corev1.Pod) bool {
	for _, container := range pod.Spec.InitContainers {
		if strings.HasPrefix(container.Name, passthroughResourcePrefix) {
			return true
		}
	}
	return false
}

// BuildInjection produces one language's passthrough mutation, or reports false when
// there is nothing to inject.
//
// A false return is not an error: a language whose container selection matches nothing in
// the pod is skipped by upstream too, and a language with no resolvable SDK image cannot
// be served at all.
func BuildInjection(request LanguageRequest, pod *corev1.Pod) (Injection, bool) {
	if pod == nil || request.Instrumentation == nil {
		return Injection{}, false
	}

	contract, ok := languagePassthroughs[request.Language]
	if !ok {
		log.Debugf("No OpenTelemetry passthrough contract for language %q", request.Language)
		return Injection{}, false
	}

	image, ok := ResolveImage(request.Instrumentation, request.Language)
	if !ok {
		log.Warnf("No OpenTelemetry SDK image for language %s of Instrumentation %s/%s, cannot inject in passthrough mode",
			request.Language, request.Instrumentation.Namespace, request.Instrumentation.Name)
		return Injection{}, false
	}

	containers := resolveContainers(pod, request.Containers)
	if len(containers) == 0 {
		log.Debugf("OpenTelemetry %s instrumentation selects no container of pod %s/%s, skipping",
			request.Language, pod.Namespace, pod.Name)
		return Injection{}, false
	}

	t := newPassthroughTelemetry()
	spec := request.Instrumentation.Spec
	reportUnsupported(spec, pod, request.Language, t)

	var (
		name = passthroughResourcePrefix + string(request.Language)
		base = passthroughMountPrefix + string(request.Language)
	)

	injection := Injection{
		Language: request.Language,
		Volume:   passthroughVolume(name, languageVolumeSizeLimit(spec, request.Language)),
		InitContainer: corev1.Container{
			Name:      name,
			Image:     image,
			Command:   contract.copyArgs(base),
			Resources: languageResources(spec, request.Language),
			VolumeMounts: []corev1.VolumeMount{{
				Name:      name,
				MountPath: base,
			}},
		},
	}

	for _, container := range containers {
		mountPath := contract.mountPath(base, container)
		envVars := contract.env(mountPath)

		// Upstream's escape hatch: a variable it would have to merge into, but which the
		// user populated from a ConfigMap or a Secret, cannot be merged at all — so
		// upstream skips the whole container rather than overwrite it. Doing anything
		// else here means either dropping the user's value or shipping a broken command
		// line.
		defined := definedEnvVars(pod, []string{container})
		if blocker, blocked := valueFromBlocker(envVars, defined); blocked {
			log.Warnf("Container %s of pod %s/%s sets %s from a reference, which OpenTelemetry %s instrumentation cannot merge into; skipping the container",
				container, pod.Namespace, pod.Name, blocker, request.Language)
			t.recordUnsupported("value_from_env", request.Language)
			continue
		}

		injection.Containers = append(injection.Containers, ContainerInjection{
			Container: container,
			Mount:     corev1.VolumeMount{Name: name, MountPath: mountPath},
			EnvVars:   append(envVars, commonConfig(spec, request.Language, pod, container, defined)...),
		})
	}

	if len(injection.Containers) == 0 {
		return Injection{}, false
	}
	return injection, true
}

// commonConfig is the configuration part of the custom resource: the env blocks the user
// wrote, plus the OpenTelemetry variables upstream derives from spec.
//
// Unlike swap mode, nothing is translated here — the values are the user's and the
// variables are the ones the community SDKs read. The custom resource's env blocks come
// first because upstream gives them precedence over its own derived values, and every
// entry is set-if-absent, so a container's own declaration always wins.
func commonConfig(
	spec otelv1alpha1.InstrumentationSpec,
	language Language,
	pod *corev1.Pod,
	container string,
	defined map[string]corev1.EnvVar,
) []EnvVar {
	// The node and pod addresses come first, and must stay first: Kubernetes expands a
	// $(VAR) reference only against variables declared earlier in the same container,
	// and the whole point of these two is that spec.exporter.endpoint can be written as
	// http://$(OTEL_NODE_IP):4318 to reach an agent running on the node. Upstream emits
	// them for the same reason.
	envVars := []EnvVar{
		{
			Name:      "OTEL_NODE_IP",
			ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.hostIP"}},
			Merge:     EnvSetIfAbsent,
		},
		{
			Name:      "OTEL_POD_IP",
			ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}},
			Merge:     EnvSetIfAbsent,
		},
	}

	// The language block outranks the common one, which is upstream's order. Built into
	// a fresh slice: appending to the one the custom resource owns would write into the
	// store's cached object, which every other pod reads.
	perLanguage := languageEnv(spec, language)
	crEnv := make([]corev1.EnvVar, 0, len(perLanguage)+len(spec.Env))
	crEnv = append(crEnv, perLanguage...)
	crEnv = append(crEnv, spec.Env...)

	for _, env := range crEnv {
		// A valueFrom entry in the custom resource cannot be expressed as a plain value.
		// Upstream copies the whole EnvVar; this mode carries values only, so such an
		// entry is dropped rather than silently emptied.
		if env.ValueFrom != nil {
			log.Debugf("Ignoring env var %s of Instrumentation: valueFrom is not supported in passthrough mode", env.Name)
			continue
		}
		envVars = append(envVars, EnvVar{Name: env.Name, Value: env.Value, Merge: EnvSetIfAbsent})
	}

	if spec.Exporter.Endpoint != "" {
		envVars = append(envVars, EnvVar{
			Name:  envOTelExporterEndpoint,
			Value: spec.Exporter.Endpoint,
			Merge: EnvSetIfAbsent,
		})
	}

	attributes := authoredResourceAttributes(spec, pod)
	if name := passthroughServiceName(spec, pod, attributes, container); name != "" {
		envVars = append(envVars, EnvVar{Name: envOTelServiceName, Value: name, Merge: EnvSetIfAbsent})
	}
	if len(spec.Propagators) > 0 {
		propagators := make([]string, 0, len(spec.Propagators))
		for _, propagator := range spec.Propagators {
			propagators = append(propagators, string(propagator))
		}
		envVars = append(envVars, EnvVar{
			Name:  envOTelPropagators,
			Value: strings.Join(propagators, ","),
			Merge: EnvSetIfAbsent,
		})
	}
	if spec.Sampler.Type != "" {
		envVars = append(envVars, EnvVar{Name: envOTelTracesSampler, Value: string(spec.Sampler.Type), Merge: EnvSetIfAbsent})
		if spec.Sampler.Argument != "" {
			envVars = append(envVars, EnvVar{Name: envOTelTracesSamplerArg, Value: spec.Sampler.Argument, Merge: EnvSetIfAbsent})
		}
	}

	// Resource attributes come last and are appended rather than set, which is upstream's
	// behaviour: a container declaring its own attributes keeps them and gains ours.
	if value := resourceAttributes(spec, pod, container, attributes, defined); value != "" {
		envVars = append(envVars, EnvVar{
			Name:      envOTelResourceAttributes,
			Value:     value,
			Separator: ",",
			Merge:     EnvAppend,
		})
	}

	return envVars
}

// resourceAttributes serialises the attributes upstream would put in
// OTEL_RESOURCE_ATTRIBUTES, restricted to those needing neither an API read nor a $(VAR)
// reference. Keys are sorted, as upstream's resourceMapToStr sorts them.
func resourceAttributes(
	spec otelv1alpha1.InstrumentationSpec,
	pod *corev1.Pod,
	container string,
	authored map[string]string,
	defined map[string]corev1.EnvVar,
) string {
	attributes := make(map[string]string, len(authored)+4)
	for key, value := range authored {
		attributes[key] = value
	}

	attributes[attrK8sNamespaceName] = pod.Namespace
	attributes[attrK8sContainerName] = container
	if version := serviceVersion(spec, pod, []string{container}, authored); version != "" {
		attributes[attrServiceVersion] = version
	}
	if _, ok := attributes[attrServiceNamespace]; !ok {
		attributes[attrServiceNamespace] = pod.Namespace
	}

	// Anything the container already declares in OTEL_RESOURCE_ATTRIBUTES is the user's,
	// and upstream never overwrites it. The value is appended to, so dropping the keys it
	// already carries is what keeps our pairs from shadowing theirs.
	for key := range parseResourceAttributes(defined[envOTelResourceAttributes].Value) {
		delete(attributes, key)
	}

	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+attributes[key])
	}
	return strings.Join(pairs, ",")
}

// parseResourceAttributes reads the keys of an OTEL_RESOURCE_ATTRIBUTES value. Malformed
// pairs are ignored rather than rejected: the variable is the user's and a parse failure
// must not stop the injection.
func parseResourceAttributes(value string) map[string]struct{} {
	if value == "" {
		return nil
	}
	keys := make(map[string]struct{})
	for _, pair := range strings.Split(value, ",") {
		if key, _, ok := strings.Cut(pair, "="); ok {
			keys[strings.TrimSpace(key)] = struct{}{}
		}
	}
	return keys
}

// passthroughServiceName resolves OTEL_SERVICE_NAME, following upstream's
// chooseServiceName as far as this package can: the resource.opentelemetry.io annotation,
// the app labels when the custom resource opted into them, the custom resource's own
// service.name attribute, and the container name as the last resort.
//
// The tiers this skips are the owner-derived ones — Deployment, StatefulSet, and the rest
// — because they need API reads. Upstream would name a Deployment's pod after the
// Deployment; this names it after its container.
func passthroughServiceName(
	spec otelv1alpha1.InstrumentationSpec,
	pod *corev1.Pod,
	attributes map[string]string,
	container string,
) string {
	if name := labelOrAnnotation(spec, pod, attrServiceName, labelAppInstance, labelAppName); name != "" {
		return name
	}
	if name := attributes[attrServiceName]; name != "" {
		return name
	}
	return container
}

// valueFromBlocker returns the first variable the injection would have to merge into but
// which the container populates from a reference.
func valueFromBlocker(envVars []EnvVar, defined map[string]corev1.EnvVar) (string, bool) {
	for _, env := range envVars {
		if env.Merge == EnvSetIfAbsent {
			continue
		}
		if existing, ok := defined[env.Name]; ok && existing.ValueFrom != nil {
			return env.Name, true
		}
	}
	return "", false
}

// passthroughVolume builds the payload volume. Upstream supports an ephemeral volume
// claim here too; see the unsupported list at the top of this file.
func passthroughVolume(name string, sizeLimit *resource.Quantity) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: sizeLimit},
		},
	}
}

// reportUnsupported logs and counts the custom resource fields and pod annotations this
// mode ignores, so that "my volumeClaimTemplate had no effect" is answerable.
func reportUnsupported(spec otelv1alpha1.InstrumentationSpec, pod *corev1.Pod, language Language, t *passthroughTelemetry) {
	if languageUsesVolumeClaim(spec, language) {
		log.Warnf("Ignoring volumeClaimTemplate of Instrumentation for language %s: passthrough mode always uses an emptyDir", language)
		t.recordUnsupported("volume_claim_template", language)
	}
	if language == Java && len(spec.Java.Extensions) > 0 {
		log.Warnf("Ignoring the %d java extension(s) of Instrumentation: passthrough mode injects no extension init container", len(spec.Java.Extensions))
		t.recordUnsupported("java_extensions", language)
	}
	if language == Python && pod.Annotations[annotationPrefix+"otel-python-platform"] == "musl" {
		log.Warnf("Ignoring the musl python platform annotation of pod %s/%s: passthrough mode always copies the glibc payload", pod.Namespace, pod.Name)
		t.recordUnsupported("musl_platform", language)
	}
	if language == DotNet && pod.Annotations[annotationPrefix+"otel-dotnet-auto-runtime"] == "linux-musl-x64" {
		log.Warnf("Ignoring the musl dotnet runtime annotation of pod %s/%s: passthrough mode always uses the glibc profiler", pod.Namespace, pod.Name)
		t.recordUnsupported("musl_runtime", language)
	}
	if spec.Resource.AddK8sUIDAttributes {
		log.Warnf("Ignoring addK8sUIDAttributes of Instrumentation: passthrough mode emits no Kubernetes UID attributes")
		t.recordUnsupported("add_k8s_uid_attributes", language)
	}
}

// languageVolumeSizeLimit returns the custom resource's volume size limit for a language,
// or upstream's default.
//
// The field is marked deprecated in favour of spec.<lang>.volume.size, which the pinned
// API version does not have yet. Switching means bumping the API module, and until then
// this is the only size the custom resource can express.
//
//nolint:staticcheck
func languageVolumeSizeLimit(spec otelv1alpha1.InstrumentationSpec, language Language) *resource.Quantity {
	var limit *resource.Quantity
	switch language {
	case Java:
		limit = spec.Java.VolumeSizeLimit
	case NodeJS:
		limit = spec.NodeJS.VolumeSizeLimit
	case Python:
		limit = spec.Python.VolumeSizeLimit
	case DotNet:
		limit = spec.DotNet.VolumeSizeLimit
	}
	if limit != nil {
		return limit
	}
	return &defaultVolumeSizeLimit
}

// languageResources returns the init container resources the custom resource asks for.
//
// Upstream's defaulting webhook fills these in when the user leaves them empty, so a
// custom resource created after that operator was replaced carries none, and the init
// container then runs without limits. That is the same defaulting gap as the SDK image,
// and unlike the image it has no table to fall back on yet.
func languageResources(spec otelv1alpha1.InstrumentationSpec, language Language) corev1.ResourceRequirements {
	switch language {
	case Java:
		return spec.Java.Resources
	case NodeJS:
		return spec.NodeJS.Resources
	case Python:
		return spec.Python.Resources
	case DotNet:
		return spec.DotNet.Resources
	}
	return corev1.ResourceRequirements{}
}

// languageUsesVolumeClaim reports whether the custom resource asked for an ephemeral
// volume claim instead of an emptyDir.
func languageUsesVolumeClaim(spec otelv1alpha1.InstrumentationSpec, language Language) bool {
	switch language {
	case Java:
		return spec.Java.VolumeClaimTemplate.Spec.Resources.Requests != nil
	case NodeJS:
		return spec.NodeJS.VolumeClaimTemplate.Spec.Resources.Requests != nil
	case Python:
		return spec.Python.VolumeClaimTemplate.Spec.Resources.Requests != nil
	case DotNet:
		return spec.DotNet.VolumeClaimTemplate.Spec.Resources.Requests != nil
	}
	return false
}
