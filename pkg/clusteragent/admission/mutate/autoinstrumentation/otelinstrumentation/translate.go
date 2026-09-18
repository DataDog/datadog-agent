// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package otelinstrumentation

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/distribution/reference"
	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DatadogLanguage is a Datadog tracing library identifier. The values match the
// autoinstrumentation package's own language identifiers, so a caller can use them to
// pick a library version and image without knowing that OpenTelemetry was involved.
type DatadogLanguage string

const (
	// DatadogJava is the Datadog Java tracing library.
	DatadogJava DatadogLanguage = "java"
	// DatadogJS is the Datadog JavaScript tracing library, which OpenTelemetry calls "nodejs".
	DatadogJS DatadogLanguage = "js"
	// DatadogPython is the Datadog Python tracing library.
	DatadogPython DatadogLanguage = "python"
	// DatadogDotNet is the Datadog .NET tracing library.
	DatadogDotNet DatadogLanguage = "dotnet"
)

// datadogLanguage maps an OpenTelemetry auto-instrumentation language to its Datadog
// tracing library. Only nodejs is renamed; the other three share a spelling.
func (l Language) datadogLanguage() (DatadogLanguage, bool) {
	switch l {
	case Java:
		return DatadogJava, true
	case NodeJS:
		return DatadogJS, true
	case Python:
		return DatadogPython, true
	case DotNet:
		return DatadogDotNet, true
	}
	return "", false
}

// LanguageConfig is one language's Datadog configuration, translated from an
// Instrumentation custom resource.
//
// It carries no library version and no image: choosing those stays with the caller,
// which already has the Datadog default-library machinery. It carries no OpenTelemetry
// type either, which is the point — the caller needs to know nothing about the custom
// resource it came from.
type LanguageConfig struct {
	// Language is the Datadog tracing library to inject.
	Language DatadogLanguage

	// Containers are the container names to instrument, resolved the way upstream
	// resolves them: the selection annotations if the pod made one, otherwise the first
	// regular container. Names that match no container in the pod are dropped, and
	// targeted init containers come first, in pod-spec order. Never empty — a language
	// whose selection resolves to nothing produces no LanguageConfig at all.
	Containers []string

	// EnvVars are the environment variables to add to each of Containers, ordered
	// highest precedence first and free of duplicate names.
	//
	// Ordering is a contract, not a detail. Values the translation computes never
	// reference another variable with $(VAR) — the k8s-derived resource attributes that
	// force upstream into that pattern are deliberately not translated (see
	// translateResourceAttributes) — so a caller may apply these in any order without
	// shipping literal $(VAR) strings. Values copied verbatim out of the custom
	// resource's env blocks are the user's own and are left in the order the custom
	// resource declared them.
	EnvVars []corev1.EnvVar
}

// Translation is a pod's swap-mode configuration: what to inject, and how to configure
// it, for every language its OpenTelemetry annotations resolved.
type Translation struct {
	// Languages holds one entry per translatable language, in MappableLanguages order.
	Languages []LanguageConfig
}

// Empty reports whether the translation asks for nothing.
func (t Translation) Empty() bool {
	return len(t.Languages) == 0
}

const (
	// resourceAttributeAnnotationPrefix is upstream's per-attribute pod annotation
	// namespace. Note it is *not* the inject-<lang> prefix.
	resourceAttributeAnnotationPrefix = "resource.opentelemetry.io/"

	// Labels upstream reads when defaults.useLabelsForResourceAttributes is set.
	labelAppInstance = "app.kubernetes.io/instance"
	labelAppName     = "app.kubernetes.io/name"
	labelAppVersion  = "app.kubernetes.io/version"

	// OpenTelemetry resource attribute keys that have a dedicated Datadog variable.
	attrServiceName = "service.name"
	// attrServiceVersion is what upstream's chooseServiceVersion feeds.
	attrServiceVersion = "service.version"
	// attrDeploymentEnvironmentName is semconv 1.27+; attrDeploymentEnvironment is the
	// 1.17 spelling it replaced. The Agent's own OTLP mapping prefers the newer one, so
	// this translation does too.
	attrDeploymentEnvironmentName = "deployment.environment.name"
	attrDeploymentEnvironment     = "deployment.environment"

	// OpenTelemetry environment variables a container can set to override the custom
	// resource. Upstream skips emitting its own value when it finds one of these, and so
	// does this translation — the Datadog tracers read a documented subset of the OTEL_*
	// variables, so the user's value keeps applying, and a DD_* equivalent would take
	// precedence over it.
	envOTelServiceName      = "OTEL_SERVICE_NAME"
	envOTelPropagators      = "OTEL_PROPAGATORS"
	envOTelTracesSampler    = "OTEL_TRACES_SAMPLER"
	envOTelTracesSamplerArg = "OTEL_TRACES_SAMPLER_ARG"

	// Datadog variables this translation produces, beyond the DD_SERVICE / DD_ENV /
	// DD_VERSION trio that pkg/util/kubernetes already names.
	envDDTags               = "DD_TAGS"
	envDDPropagationStyle   = "DD_TRACE_PROPAGATION_STYLE"
	envDDTraceSampleRate    = "DD_TRACE_SAMPLE_RATE"
	envDDTraceSamplingRules = "DD_TRACE_SAMPLING_RULES"
)

// Translate turns a resolved Result into Datadog configuration, one entry per language.
//
// Swap mode reads the Instrumentation custom resource for its *configuration* and
// injects the Datadog tracing library configured to match, so nothing OpenTelemetry
// specific survives in the output. A field that has no Datadog equivalent is dropped
// with a log line and a telemetry counter rather than approximated, and never fails the
// translation: leaving a pod uninstrumented over an unrepresentable sampler would be a
// far worse outcome than instrumenting it with the tracer's own default.
//
// Only languages resolved to ModeDatadog are translated. A language resolved to
// passthrough is skipped here and served by BuildInjections instead: swapping a language
// whose custom resource deliberately names a community SDK image would inject the one
// thing the user asked us not to.
//
// A non-resolved Result translates to nothing, so callers can hand any Result over
// without branching first.
func Translate(result Result, pod *corev1.Pod) Translation {
	if result.Outcome != OutcomeResolved || pod == nil {
		return Translation{}
	}

	t := newTranslationTelemetry()
	configs := make([]LanguageConfig, 0, len(result.Languages))
	for _, request := range result.Languages {
		if request.Mode != ModeDatadog {
			// Not a warning: this is the normal path for a passthrough language, which
			// BuildInjections serves with the community SDK.
			log.Debugf("OpenTelemetry %s instrumentation of pod %s/%s resolved to mode %q (%s), leaving it to passthrough",
				request.Language, pod.Namespace, pod.Name, request.Mode, request.ModeSource)
			continue
		}

		language, ok := request.Language.datadogLanguage()
		if !ok {
			// Unreachable while MappableLanguages and datadogLanguage agree, but a
			// language added to one and not the other must not be injected as an empty
			// library name.
			log.Debugf("No Datadog tracing library for OpenTelemetry language %q, skipping", request.Language)
			continue
		}

		containers := resolveContainers(pod, request.Containers)
		if len(containers) == 0 {
			// Upstream's containersToInstrument returns nothing and its caller skips the
			// language entirely.
			log.Debugf("OpenTelemetry %s instrumentation selects no container of pod %s/%s, skipping",
				request.Language, pod.Namespace, pod.Name)
			continue
		}

		configs = append(configs, LanguageConfig{
			Language:   language,
			Containers: containers,
			EnvVars:    translateEnvVars(request, language, pod, containers, t),
		})
	}

	if len(configs) == 0 {
		return Translation{}
	}
	return Translation{Languages: configs}
}

// translateEnvVars builds one language's environment, layered the way upstream layers
// it and ordered so that the highest precedence entry comes first.
//
// Upstream applies every one of these with appendIfNotSet, giving the precedence its CRD
// documents: `original container env vars` > `language specific env vars` >
// `common env vars` > `instrument spec configs' vars`. Reproducing it here needs no
// merge logic, only the same order plus first-wins deduplication, because the container
// tier is enforced by dropping anything the container already sets.
func translateEnvVars(
	request LanguageRequest,
	language DatadogLanguage,
	pod *corev1.Pod,
	containers []string,
	t *translationTelemetry,
) []corev1.EnvVar {
	spec := request.Instrumentation.Spec
	defined := definedEnvVars(pod, containers)

	var envVars []corev1.EnvVar
	envVars = append(envVars, languageEnv(spec, request.Language)...)
	envVars = append(envVars, spec.Env...)
	envVars = append(envVars, translateSpecConfig(request, language, pod, containers, defined, t)...)

	// Upstream's escape hatch, and the parent package's: a variable the container
	// already declares is the user's, and we never take it over. The check is "any
	// selected container", not "all", because one environment is applied to every
	// selected container and partially overriding a user's explicit value is worse than
	// deferring to it. The caller's own env var mutators refuse to overwrite too, so
	// this is a first guard rather than the only one.
	out := make([]corev1.EnvVar, 0, len(envVars))
	seen := make(map[string]struct{}, len(envVars))
	for _, env := range envVars {
		if _, ok := defined[env.Name]; ok {
			log.Debugf("Container of pod %s/%s already sets %s, leaving it alone", pod.Namespace, pod.Name, env.Name)
			continue
		}
		if _, ok := seen[env.Name]; ok {
			continue
		}
		seen[env.Name] = struct{}{}
		out = append(out, env)
	}
	return out
}

// languageEnv returns the per-language env block of the custom resource.
func languageEnv(spec otelv1alpha1.InstrumentationSpec, language Language) []corev1.EnvVar {
	switch language {
	case Java:
		return spec.Java.Env
	case NodeJS:
		return spec.NodeJS.Env
	case Python:
		return spec.Python.Env
	case DotNet:
		return spec.DotNet.Env
	}
	return nil
}

// translateSpecConfig maps the custom resource's configuration fields — exporter,
// resource attributes, propagators, sampler — onto Datadog variables.
func translateSpecConfig(
	request LanguageRequest,
	language DatadogLanguage,
	pod *corev1.Pod,
	containers []string,
	defined map[string]corev1.EnvVar,
	t *translationTelemetry,
) []corev1.EnvVar {
	spec := request.Instrumentation.Spec

	// exporter.endpoint and its TLS material are deliberately not translated. A Datadog
	// tracer reaches the local Trace Agent on its own, so pointing it at the collector
	// endpoint the customer configured for the community SDKs would send traces
	// somewhere that does not speak the Datadog protocol. Dropping it is the whole
	// point of swap mode, but it is dropped loudly so that a surprised customer can see
	// why their endpoint had no effect.
	if spec.Exporter.Endpoint != "" {
		log.Debugf("Ignoring exporter.endpoint %q of Instrumentation %s/%s: Datadog tracers reach the local Trace Agent natively",
			spec.Exporter.Endpoint, request.Instrumentation.Namespace, request.Instrumentation.Name)
		t.recordDropped("exporter_endpoint", "set")
	}

	var envVars []corev1.EnvVar
	envVars = append(envVars, translateResourceAttributes(spec, pod, containers, defined)...)
	envVars = append(envVars, translatePropagators(spec.Propagators, language, defined, t)...)
	envVars = append(envVars, translateSampler(spec.Sampler, defined, t)...)
	return envVars
}

// translateResourceAttributes maps the resource attributes and the service name /
// version derivation onto DD_SERVICE, DD_ENV, DD_VERSION and DD_TAGS.
//
// Only attributes a human wrote are translated: the custom resource's
// resource.resourceAttributes and the pod's resource.opentelemetry.io/<attr>
// annotations, the latter overriding the former per key as upstream does. Upstream's
// Kubernetes-derived attributes (k8s.pod.name, k8s.deployment.name, service.instance.id,
// …) are deliberately dropped: the Datadog Agent's tagger already attaches the
// equivalent tags to every span it receives, so translating them would restate what the
// platform supplies anyway. Dropping them is also what keeps this translation free of
// $(VAR) references, since upstream's values for k8s.pod.name and k8s.node.name are
// downward-API references that only expand when declared in the right order.
func translateResourceAttributes(
	spec otelv1alpha1.InstrumentationSpec,
	pod *corev1.Pod,
	containers []string,
	defined map[string]corev1.EnvVar,
) []corev1.EnvVar {
	attributes := authoredResourceAttributes(spec, pod)

	var envVars []corev1.EnvVar
	if service := serviceName(spec, pod, attributes, defined); service != nil {
		envVars = append(envVars, *service)
	}
	if env := deploymentEnvironment(attributes); env != "" {
		envVars = append(envVars, corev1.EnvVar{Name: kubernetes.EnvTagEnvVar, Value: env})
	}
	if version := serviceVersion(spec, pod, containers, attributes); version != "" {
		envVars = append(envVars, corev1.EnvVar{Name: kubernetes.VersionTagEnvVar, Value: version})
	}
	if tags := remainingAttributesAsTags(attributes); tags != "" {
		envVars = append(envVars, corev1.EnvVar{Name: envDDTags, Value: tags})
	}
	return envVars
}

// authoredResourceAttributes merges the two attribute sources a human writes. The
// custom resource is the lower tier and the pod annotations override it per key, which
// is upstream's ordering in createResourceMap.
func authoredResourceAttributes(spec otelv1alpha1.InstrumentationSpec, pod *corev1.Pod) map[string]string {
	attributes := make(map[string]string, len(spec.Resource.Attributes)+len(pod.Annotations))
	for key, value := range spec.Resource.Attributes {
		attributes[key] = value
	}
	for key, value := range pod.Annotations {
		attribute, ok := strings.CutPrefix(key, resourceAttributeAnnotationPrefix)
		if !ok || attribute == "" {
			continue
		}
		attributes[attribute] = value
	}
	return attributes
}

// serviceName resolves DD_SERVICE, or nil to leave it to the caller.
//
// The order follows upstream's chooseServiceName as far as it can be followed from the
// pod and its namespace alone:
//
//  1. a container's own OTEL_SERVICE_NAME, which upstream never overwrites and which
//     the OpenTelemetry SDK gives precedence over any service.name resource attribute.
//     Copied whole, so a value sourced from the downward API or a secret keeps working.
//  2. the resource.opentelemetry.io/service.name pod annotation.
//  3. app.kubernetes.io/instance then app.kubernetes.io/name, when the custom resource
//     opted in with defaults.useLabelsForResourceAttributes.
//  4. the custom resource's own service.name resource attribute.
//
// Returning nil past that point is deliberate. Upstream's remaining tiers are the owner
// workload's name, then the pod name, then the container name, and it walks owner
// references to reach the first of those, with a cache-backed Get for ReplicaSet →
// Deployment and Job → CronJob. The autoinstrumentation package already derives
// DD_SERVICE from the pod's owner references, parsing the Deployment name out of the
// ReplicaSet name, so leaving DD_SERVICE unset here yields the same meaning through
// machinery that is already in place.
//
// Tier 4 is a deliberate divergence: upstream reads service.name only from the
// annotation and the labels, so a service.name written in the custom resource's
// resourceAttributes reaches OTEL_RESOURCE_ATTRIBUTES but is then shadowed by
// OTEL_SERVICE_NAME, which chooseServiceName always fills (its last resort is the
// container's name). Honouring it costs nothing and beats silently discarding
// configuration the user wrote.
func serviceName(
	spec otelv1alpha1.InstrumentationSpec,
	pod *corev1.Pod,
	attributes map[string]string,
	defined map[string]corev1.EnvVar,
) *corev1.EnvVar {
	if existing, ok := defined[envOTelServiceName]; ok {
		return &corev1.EnvVar{
			Name:      kubernetes.ServiceTagEnvVar,
			Value:     existing.Value,
			ValueFrom: existing.ValueFrom,
		}
	}

	if name := labelOrAnnotation(spec, pod, attrServiceName, labelAppInstance, labelAppName); name != "" {
		return &corev1.EnvVar{Name: kubernetes.ServiceTagEnvVar, Value: name}
	}
	if name := attributes[attrServiceName]; name != "" {
		return &corev1.EnvVar{Name: kubernetes.ServiceTagEnvVar, Value: name}
	}
	return nil
}

// serviceVersion resolves DD_VERSION, mirroring upstream's chooseServiceVersion: the
// annotation, then the app.kubernetes.io/version label when the custom resource opted
// into labels, then the container image's tag and digest. The custom resource's own
// service.version attribute is last, which is also where upstream leaves it — it only
// survives when the image reference carries neither tag nor digest.
//
// The image is read from the first selected container. Upstream computes the version per
// container and could therefore derive a different one for each; a single Datadog
// environment is applied to every selected container, so a pod that selects containers
// running different images gets the first one's version.
func serviceVersion(
	spec otelv1alpha1.InstrumentationSpec,
	pod *corev1.Pod,
	containers []string,
	attributes map[string]string,
) string {
	if version := labelOrAnnotation(spec, pod, attrServiceVersion, labelAppVersion); version != "" {
		return version
	}
	if container := findContainer(pod, containers[0]); container != nil {
		if version := versionFromImage(container.Image); version != "" {
			return version
		}
	}
	return attributes[attrServiceVersion]
}

// deploymentEnvironment resolves DD_ENV. OpenTelemetry has no notion of a deployment
// environment outside the semantic conventions, so the only sources are the resource
// attributes; deployment.environment.name is semconv 1.27+ and supersedes the older
// deployment.environment, matching how the Agent maps OTLP resources.
func deploymentEnvironment(attributes map[string]string) string {
	if env := attributes[attrDeploymentEnvironmentName]; env != "" {
		return env
	}
	return attributes[attrDeploymentEnvironment]
}

// remainingAttributesAsTags renders every attribute that has no dedicated Datadog
// variable as DD_TAGS. Keys are sorted, as upstream sorts OTEL_RESOURCE_ATTRIBUTES, so
// that the same custom resource always produces the same pod spec.
func remainingAttributesAsTags(attributes map[string]string) string {
	consumed := map[string]struct{}{
		attrServiceName:               {},
		attrServiceVersion:            {},
		attrDeploymentEnvironmentName: {},
		attrDeploymentEnvironment:     {},
	}

	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		if _, ok := consumed[key]; ok {
			continue
		}
		if attributes[key] == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	tags := make([]string, 0, len(keys))
	for _, key := range keys {
		tags = append(tags, key+":"+attributes[key])
	}
	return strings.Join(tags, ",")
}

// labelOrAnnotation is upstream's chooseLabelOrAnnotation: the
// resource.opentelemetry.io/<attribute> annotation wins, then the given labels in order
// but only when the custom resource set defaults.useLabelsForResourceAttributes.
func labelOrAnnotation(spec otelv1alpha1.InstrumentationSpec, pod *corev1.Pod, attribute string, labels ...string) string {
	if value := pod.Annotations[resourceAttributeAnnotationPrefix+attribute]; value != "" {
		return value
	}
	if !spec.Defaults.UseLabelsForResourceAttributes {
		return ""
	}
	for _, label := range labels {
		if value := pod.Labels[label]; value != "" {
			return value
		}
	}
	return ""
}

// versionFromImage extracts a version from a container image reference the way
// upstream's parseServiceVersionFromImage does: "<tag>@<digest>" when the reference
// carries both, otherwise the digest, otherwise the tag, otherwise nothing.
func versionFromImage(image string) string {
	if image == "" {
		return ""
	}
	ref, err := reference.Parse(image)
	if err != nil {
		log.Debugf("Cannot parse image reference %q for a service version: %v", image, err)
		return ""
	}
	named, ok := ref.(reference.Named)
	if !ok {
		return ""
	}

	var tag, digest string
	if tagged, ok := named.(reference.Tagged); ok {
		tag = tagged.Tag()
	}
	if digested, ok := named.(reference.Digested); ok {
		digest = digested.Digest().String()
	}

	switch {
	case digest != "" && tag != "":
		return tag + "@" + digest
	case digest != "":
		return digest
	default:
		return tag
	}
}

// translatePropagators maps spec.propagators onto DD_TRACE_PROPAGATION_STYLE.
//
// Propagators with no Datadog equivalent are dropped with a log line rather than failing
// the translation, because the alternative is to leave a pod uninstrumented over a
// header format — and a pod that traces with the wrong propagation is still far more
// useful than a pod that does not trace. If *every* propagator is unmappable the
// variable is left unset so the tracer keeps its own default, since an empty
// DD_TRACE_PROPAGATION_STYLE reads as "no styles" and would silently break distributed
// tracing altogether.
//
// A container that already sets OTEL_PROPAGATORS has overridden the custom resource for
// upstream too, and the Datadog tracers honour that variable as an alias, so nothing is
// translated for such a container: DD_TRACE_PROPAGATION_STYLE would outrank the value
// the user asked for.
func translatePropagators(
	propagators []otelv1alpha1.Propagator,
	language DatadogLanguage,
	defined map[string]corev1.EnvVar,
	t *translationTelemetry,
) []corev1.EnvVar {
	if len(propagators) == 0 {
		return nil
	}
	if _, ok := defined[envOTelPropagators]; ok {
		log.Debugf("Container already sets %s, not translating the custom resource's propagators", envOTelPropagators)
		return nil
	}

	styles := make([]string, 0, len(propagators))
	seen := make(map[string]struct{}, len(propagators))
	for _, propagator := range propagators {
		style, ok := propagationStyle(propagator, language)
		if !ok {
			log.Debugf("OpenTelemetry propagator %q has no %s equivalent, dropping it", propagator, language)
			t.recordDropped("propagator", string(propagator))
			continue
		}
		if _, dup := seen[style]; dup {
			continue
		}
		seen[style] = struct{}{}
		styles = append(styles, style)
	}

	if len(styles) == 0 {
		log.Debugf("No OpenTelemetry propagator has a %s equivalent, leaving %s unset", language, envDDPropagationStyle)
		return nil
	}
	return []corev1.EnvVar{{Name: envDDPropagationStyle, Value: strings.Join(styles, ",")}}
}

// propagationStyle maps one OpenTelemetry propagator to a Datadog propagation style.
//
// Two of the eight need per-language care. B3 single-header has no single spelling
// across the Datadog tracers, and picking the wrong one is not a no-op: dd-trace-java
// reads "b3" as the *multi*-header style, so the obvious mapping would silently change
// the wire format. AWS X-Ray is only implemented by dd-trace-java. jaeger and ottrace
// have no Datadog implementation at all.
func propagationStyle(propagator otelv1alpha1.Propagator, language DatadogLanguage) (string, bool) {
	switch propagator {
	case otelv1alpha1.TraceContext:
		return "tracecontext", true
	case otelv1alpha1.Baggage:
		return "baggage", true
	case otelv1alpha1.B3Multi:
		return "b3multi", true
	case otelv1alpha1.B3:
		return b3SinglePropagationStyle(language), true
	case otelv1alpha1.XRay:
		if language == DatadogJava {
			return "xray", true
		}
		return "", false
	case otelv1alpha1.None:
		// OpenTelemetry's "none" disables propagation, and so does Datadog's.
		return "none", true
	case otelv1alpha1.Jaeger, otelv1alpha1.OTTrace:
		return "", false
	}
	return "", false
}

// b3SinglePropagationStyle returns the value each Datadog tracer uses for B3
// single-header propagation. "B3 single header" is accepted by the JavaScript and .NET
// tracers, dd-trace-java spells it "b3 single header", and dd-trace-py dropped that
// spelling in v3 in favour of a bare "b3".
func b3SinglePropagationStyle(language DatadogLanguage) string {
	switch language {
	case DatadogJava:
		return "b3 single header"
	case DatadogPython:
		return "b3"
	default:
		return "B3 single header"
	}
}

// translateSampler maps spec.sampler onto a Datadog sampling rate.
//
// Every OpenTelemetry sampler that expresses a head-based probability becomes that
// probability. The parentbased_* variants map to the same rate as their root sampler
// because a Datadog tracer is parent-based by construction: it honours an incoming
// sampling decision and only samples when it is the root of the trace.
//
// The rate is written to both DD_TRACE_SAMPLE_RATE and DD_TRACE_SAMPLING_RULES because
// no single variable covers all four tracers: DD_TRACE_SAMPLE_RATE is what the Java,
// JavaScript and .NET documentation names for a global rate, while dd-trace-py removed
// it in v3 and only reads a catch-all DD_TRACE_SAMPLING_RULES entry. Both carry the same
// rate, so which one a tracer honours cannot change the outcome.
//
// Neither variable is emitted when the container already sets either of them, or either
// of OTEL_TRACES_SAMPLER / OTEL_TRACES_SAMPLER_ARG. The joint condition is upstream's:
// it emits no sampler configuration at all if the container names one half of the pair,
// so that a half-configured sampler cannot result. Ours needs the same treatment for a
// sharper reason — a rate and a rule set that disagree do not merge, the rules win for
// matching traces and the rate becomes a catch-all for the rest.
func translateSampler(sampler otelv1alpha1.Sampler, defined map[string]corev1.EnvVar, t *translationTelemetry) []corev1.EnvVar {
	if sampler.Type == "" {
		return nil
	}

	for _, name := range []string{envDDTraceSampleRate, envDDTraceSamplingRules, envOTelTracesSampler, envOTelTracesSamplerArg} {
		if _, ok := defined[name]; ok {
			log.Debugf("Container already sets %s, not translating sampler %q", name, sampler.Type)
			return nil
		}
	}

	rate, ok := samplingRate(sampler, t)
	if !ok {
		return nil
	}

	// 'g' with -1 precision keeps 1 as "1" and 0.25 as "0.25" rather than padding to a
	// fixed width, which matters only because the value ends up in a pod spec a human
	// reads.
	value := strconv.FormatFloat(rate, 'g', -1, 64)
	return []corev1.EnvVar{
		{Name: envDDTraceSampleRate, Value: value},
		{Name: envDDTraceSamplingRules, Value: fmt.Sprintf(`[{"sample_rate":%s}]`, value)},
	}
}

// samplingRate turns a sampler type and argument into a probability.
func samplingRate(sampler otelv1alpha1.Sampler, t *translationTelemetry) (float64, bool) {
	switch sampler.Type {
	case otelv1alpha1.AlwaysOn, otelv1alpha1.ParentBasedAlwaysOn:
		return 1, true

	case otelv1alpha1.AlwaysOff, otelv1alpha1.ParentBasedAlwaysOff:
		// A rate of 0 rather than DD_TRACE_ENABLED=false: always_off stops traces from
		// being sampled, it does not stop the SDK, and disabling the tracer outright
		// would also drop the profiling, telemetry and runtime metrics that a Datadog
		// tracer carries alongside spans.
		return 0, true

	case otelv1alpha1.TraceIDRatio, otelv1alpha1.ParentBasedTraceIDRatio:
		rate, err := strconv.ParseFloat(sampler.Argument, 64)
		if err != nil || rate < 0 || rate > 1 {
			// Guessing a rate would silently change how much of a customer's traffic is
			// billed, so an unusable argument means no sampling configuration and the
			// tracer's default instead.
			log.Warnf("OpenTelemetry sampler %q has an unusable argument %q, ignoring the sampler configuration",
				sampler.Type, sampler.Argument)
			t.recordDropped("sampler_argument", string(sampler.Type))
			return 0, false
		}
		return rate, true

	case otelv1alpha1.JaegerRemote, otelv1alpha1.ParentBasedJaegerRemote:
		// Both fetch their rate from a Jaeger remote-sampling endpoint. Datadog has its
		// own remote sampling, driven by the Agent and the Datadog backend, and it is
		// what the tracer does when no rate is configured — so emitting nothing is not
		// only the safe answer here, it is the closest one.
		log.Infof("OpenTelemetry sampler %q cannot be expressed as a Datadog sampling rate; the tracer's Agent-driven remote sampling applies instead",
			sampler.Type)
		t.recordDropped("sampler", string(sampler.Type))
		return 0, false

	case otelv1alpha1.XRaySampler:
		// AWS X-Ray centralized sampling: the rate lives in an X-Ray sampling rule the
		// SDK polls, so there is nothing to translate.
		log.Infof("OpenTelemetry sampler %q cannot be expressed as a Datadog sampling rate, ignoring it", sampler.Type)
		t.recordDropped("sampler", string(sampler.Type))
		return 0, false
	}

	// The CRD constrains the field with an enum, but a cluster can hold a custom
	// resource written before the enum was tightened, or one applied with validation
	// disabled.
	log.Warnf("Unknown OpenTelemetry sampler type %q, ignoring the sampler configuration", sampler.Type)
	t.recordDropped("sampler", "unknown")
	return 0, false
}

// resolveContainers turns a language's container selection into the container names to
// instrument, mirroring upstream's containersToInstrument: an empty selection means the
// first regular container, names matching no container are dropped, and selected init
// containers come first in pod-spec order.
func resolveContainers(pod *corev1.Pod, selected []string) []string {
	if len(selected) == 0 {
		if len(pod.Spec.Containers) == 0 {
			return nil
		}
		return []string{pod.Spec.Containers[0].Name}
	}

	initContainers := make([]string, 0, len(selected))
	regularContainers := make([]string, 0, len(selected))
	for _, name := range selected {
		switch {
		case containerIndex(pod.Spec.InitContainers, name) >= 0:
			initContainers = append(initContainers, name)
		case containerIndex(pod.Spec.Containers, name) >= 0:
			regularContainers = append(regularContainers, name)
		}
	}
	sort.SliceStable(initContainers, func(i, j int) bool {
		return containerIndex(pod.Spec.InitContainers, initContainers[i]) <
			containerIndex(pod.Spec.InitContainers, initContainers[j])
	})
	return append(initContainers, regularContainers...)
}

// definedEnvVars indexes the environment variables the selected containers already
// declare. A name present on any of them counts as defined.
func definedEnvVars(pod *corev1.Pod, containers []string) map[string]corev1.EnvVar {
	defined := make(map[string]corev1.EnvVar)
	for _, name := range containers {
		container := findContainer(pod, name)
		if container == nil {
			continue
		}
		for _, env := range container.Env {
			if _, ok := defined[env.Name]; !ok {
				defined[env.Name] = env
			}
		}
	}
	return defined
}

// findContainer looks a container up by name among the regular containers and then the
// init containers, which is the order upstream searches in.
func findContainer(pod *corev1.Pod, name string) *corev1.Container {
	if i := containerIndex(pod.Spec.Containers, name); i >= 0 {
		return &pod.Spec.Containers[i]
	}
	if i := containerIndex(pod.Spec.InitContainers, name); i >= 0 {
		return &pod.Spec.InitContainers[i]
	}
	return nil
}

func containerIndex(containers []corev1.Container, name string) int {
	for i := range containers {
		if containers[i].Name == name {
			return i
		}
	}
	return -1
}
