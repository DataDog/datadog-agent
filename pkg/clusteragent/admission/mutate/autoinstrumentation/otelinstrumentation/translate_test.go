// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package otelinstrumentation

import (
	"strings"
	"testing"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// translatePod is a minimal pod. Annotations and labels are pre-allocated so tests can
// add to them without a nil map check, and the image carries no tag so that a test only
// gets a DD_VERSION when it asks for one.
func translatePod(containerNames ...string) *corev1.Pod {
	if len(containerNames) == 0 {
		containerNames = []string{"app"}
	}
	containers := make([]corev1.Container, 0, len(containerNames))
	for _, name := range containerNames {
		containers = append(containers, corev1.Container{Name: name, Image: "registry/app"})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "workloads",
			Name:        "app-7d9f8-abcde",
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
		Spec: corev1.PodSpec{Containers: containers},
	}
}

// resolvedFor builds the Result the resolver would produce for a single language.
func resolvedFor(language Language, spec otelv1alpha1.InstrumentationSpec, containers ...string) Result {
	return Result{
		Outcome: OutcomeResolved,
		Languages: []LanguageRequest{{
			Language: language,
			Instrumentation: &otelv1alpha1.Instrumentation{
				ObjectMeta: metav1.ObjectMeta{Namespace: "workloads", Name: "default"},
				Spec:       spec,
			},
			Containers: containers,
			// Swap is what Translate serves; the mode decision itself is covered by
			// mode_test.go, and a passthrough language never reaches here.
			Mode:       ModeDatadog,
			ModeSource: ModeSourceNoImage,
		}},
	}
}

// envValue returns the value of name in the first (and, in these tests, only) language
// config, plus whether it was emitted at all.
func envValue(t *testing.T, translation Translation, name string) (string, bool) {
	t.Helper()
	require.Len(t, translation.Languages, 1)
	for _, env := range translation.Languages[0].EnvVars {
		if env.Name == name {
			return env.Value, true
		}
	}
	return "", false
}

func envNames(translation Translation) []string {
	if len(translation.Languages) == 0 {
		return nil
	}
	names := make([]string, 0, len(translation.Languages[0].EnvVars))
	for _, env := range translation.Languages[0].EnvVars {
		names = append(names, env.Name)
	}
	return names
}

func TestTranslateLanguageMapping(t *testing.T) {
	// Every mappable OpenTelemetry language must reach a Datadog library, and nodejs is
	// the only one that is renamed.
	expected := map[Language]DatadogLanguage{
		Java:   DatadogJava,
		NodeJS: DatadogJS,
		Python: DatadogPython,
		DotNet: DatadogDotNet,
	}
	require.Len(t, expected, len(MappableLanguages))

	for _, language := range MappableLanguages {
		t.Run(string(language), func(t *testing.T) {
			translation := Translate(resolvedFor(language, otelv1alpha1.InstrumentationSpec{}), translatePod())
			require.Len(t, translation.Languages, 1)
			assert.Equal(t, expected[language], translation.Languages[0].Language)
			assert.Equal(t, []string{"app"}, translation.Languages[0].Containers)
		})
	}
}

func TestTranslateAllLanguagesAtOnce(t *testing.T) {
	// A pod may resolve several languages, each against its own custom resource.
	spec := otelv1alpha1.InstrumentationSpec{
		Sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.AlwaysOn},
	}
	result := Result{Outcome: OutcomeResolved}
	for _, language := range MappableLanguages {
		result.Languages = append(result.Languages, resolvedFor(language, spec).Languages...)
	}

	translation := Translate(result, translatePod())
	require.Len(t, translation.Languages, 4)
	assert.Equal(t,
		[]DatadogLanguage{DatadogJava, DatadogJS, DatadogPython, DatadogDotNet},
		[]DatadogLanguage{
			translation.Languages[0].Language,
			translation.Languages[1].Language,
			translation.Languages[2].Language,
			translation.Languages[3].Language,
		})
}

// A language resolved to passthrough must be skipped rather than swapped. Swapping it
// would inject a Datadog library over a community SDK image the user chose on purpose,
// which is the one outcome the mode discriminator exists to prevent.
func TestTranslateSkipsLanguagesNotInSwapMode(t *testing.T) {
	for _, mode := range []Mode{ModeOTel, ModeDisabled} {
		t.Run(string(mode), func(t *testing.T) {
			result := resolvedFor(Java, otelv1alpha1.InstrumentationSpec{})
			result.Languages[0].Mode = mode
			result.Languages[0].ModeSource = ModeSourceUserImage

			assert.True(t, Translate(result, translatePod()).Empty())
		})
	}
}

func TestTranslateMixedModes(t *testing.T) {
	// One custom resource pinning a community SDK image for python only: java is still
	// swapped, and python is left alone. Injecting one language and skipping the other
	// is deliberate — the annotations are per language, so a pod asking for both is
	// asking for two independent things.
	spec := otelv1alpha1.InstrumentationSpec{}
	result := Result{Outcome: OutcomeResolved}
	result.Languages = append(result.Languages, resolvedFor(Java, spec).Languages...)

	python := resolvedFor(Python, spec).Languages[0]
	python.Mode = ModeOTel
	python.ModeSource = ModeSourceUnknownImage
	result.Languages = append(result.Languages, python)

	translation := Translate(result, translatePod())
	require.Len(t, translation.Languages, 1)
	assert.Equal(t, DatadogJava, translation.Languages[0].Language)
}

func TestTranslateNonResolvedResult(t *testing.T) {
	for _, outcome := range []Outcome{OutcomeNoAnnotation, OutcomeUnresolvable} {
		t.Run(string(outcome), func(t *testing.T) {
			translation := Translate(Result{Outcome: outcome}, translatePod())
			assert.True(t, translation.Empty())
		})
	}
}

func TestTranslateExporterEndpointIsDropped(t *testing.T) {
	// Swap mode ignores the endpoint entirely: a Datadog tracer reaches the Trace Agent
	// on its own, and there is no DD_ variable that should carry the collector address.
	spec := otelv1alpha1.InstrumentationSpec{
		Exporter: otelv1alpha1.Exporter{
			Endpoint: "http://otel-collector.observability:4317",
			TLS:      &otelv1alpha1.TLS{SecretName: "certs", CA: "ca.crt"},
		},
	}

	translation := Translate(resolvedFor(Java, spec), translatePod())
	require.Len(t, translation.Languages, 1)
	// The endpoint and its TLS material are the whole spec here, so nothing is left to
	// translate: no variable carries the collector address, under any name.
	assert.Empty(t, translation.Languages[0].EnvVars)
}

func TestTranslatePropagators(t *testing.T) {
	tests := []struct {
		name        string
		language    Language
		propagators []otelv1alpha1.Propagator
		expected    string
		unset       bool
	}{
		{
			name:        "tracecontext and baggage",
			language:    Java,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.TraceContext, otelv1alpha1.Baggage},
			expected:    "tracecontext,baggage",
		},
		{
			name:        "b3multi",
			language:    Java,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.B3Multi},
			expected:    "b3multi",
		},
		{
			// dd-trace-java reads a bare "b3" as the multi-header style, so B3 single
			// has to be spelled out or the wire format silently changes.
			name:        "b3 single header on java",
			language:    Java,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.B3},
			expected:    "b3 single header",
		},
		{
			// dd-trace-py removed the "b3 single header" spelling in v3.
			name:        "b3 single header on python",
			language:    Python,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.B3},
			expected:    "b3",
		},
		{
			name:        "b3 single header on js",
			language:    NodeJS,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.B3},
			expected:    "B3 single header",
		},
		{
			name:        "b3 single header on dotnet",
			language:    DotNet,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.B3},
			expected:    "B3 single header",
		},
		{
			name:        "none",
			language:    Java,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.None},
			expected:    "none",
		},
		{
			// Only dd-trace-java implements X-Ray propagation.
			name:        "xray on java",
			language:    Java,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.XRay},
			expected:    "xray",
		},
		{
			name:        "xray elsewhere is dropped",
			language:    Python,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.XRay},
			unset:       true,
		},
		{
			name:        "jaeger is dropped",
			language:    Java,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.Jaeger},
			unset:       true,
		},
		{
			name:        "ottrace is dropped",
			language:    Java,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.OTTrace},
			unset:       true,
		},
		{
			// An unmappable propagator must not take the mappable ones down with it.
			name:     "unmappable ones are dropped, the rest survive",
			language: Java,
			propagators: []otelv1alpha1.Propagator{
				otelv1alpha1.Jaeger, otelv1alpha1.TraceContext, otelv1alpha1.OTTrace, otelv1alpha1.B3Multi,
			},
			expected: "tracecontext,b3multi",
		},
		{
			// Nothing mappable leaves the variable unset so the tracer keeps its own
			// default; an empty value would read as "propagation disabled".
			name:        "nothing mappable leaves the style unset",
			language:    Python,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.Jaeger, otelv1alpha1.OTTrace, otelv1alpha1.XRay},
			unset:       true,
		},
		{
			name:        "no propagators at all",
			language:    Java,
			propagators: nil,
			unset:       true,
		},
		{
			// Two OpenTelemetry propagators can collapse onto one Datadog style only
			// through duplicates in the custom resource; the value must stay clean.
			name:        "duplicates are collapsed",
			language:    Java,
			propagators: []otelv1alpha1.Propagator{otelv1alpha1.TraceContext, otelv1alpha1.TraceContext},
			expected:    "tracecontext",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := otelv1alpha1.InstrumentationSpec{Propagators: test.propagators}
			translation := Translate(resolvedFor(test.language, spec), translatePod())

			value, found := envValue(t, translation, envDDPropagationStyle)
			if test.unset {
				assert.False(t, found, "expected %s to be left unset, got %q", envDDPropagationStyle, value)
				return
			}
			require.True(t, found, "expected %s to be set", envDDPropagationStyle)
			assert.Equal(t, test.expected, value)
		})
	}
}

func TestTranslatePropagatorsSuppressedByContainerOverride(t *testing.T) {
	// A container that sets either the OpenTelemetry variable or the Datadog one has
	// overridden the custom resource, and DD_TRACE_PROPAGATION_STYLE would outrank the
	// OTEL_PROPAGATORS alias the tracers honour.
	spec := otelv1alpha1.InstrumentationSpec{Propagators: []otelv1alpha1.Propagator{otelv1alpha1.B3Multi}}

	for _, name := range []string{envOTelPropagators, envDDPropagationStyle} {
		t.Run(name, func(t *testing.T) {
			pod := translatePod()
			pod.Spec.Containers[0].Env = []corev1.EnvVar{{Name: name, Value: "datadog"}}

			translation := Translate(resolvedFor(Java, spec), pod)
			_, found := envValue(t, translation, envDDPropagationStyle)
			assert.False(t, found)
		})
	}
}

func TestTranslateSampler(t *testing.T) {
	tests := []struct {
		name    string
		sampler otelv1alpha1.Sampler
		rate    string
		unset   bool
	}{
		{
			name:    "always_on",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.AlwaysOn},
			rate:    "1",
		},
		{
			name:    "always_off",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.AlwaysOff},
			rate:    "0",
		},
		{
			name:    "traceidratio",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.TraceIDRatio, Argument: "0.25"},
			rate:    "0.25",
		},
		{
			// A Datadog tracer is parent-based by construction, so the parentbased_*
			// variants carry the same rate as their root sampler.
			name:    "parentbased_always_on",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.ParentBasedAlwaysOn},
			rate:    "1",
		},
		{
			name:    "parentbased_always_off",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.ParentBasedAlwaysOff},
			rate:    "0",
		},
		{
			name:    "parentbased_traceidratio",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.ParentBasedTraceIDRatio, Argument: "0.1"},
			rate:    "0.1",
		},
		{
			// The rate lives on a Jaeger remote-sampling endpoint; Datadog's own
			// Agent-driven remote sampling is what applies when nothing is set.
			name:    "jaeger_remote",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.JaegerRemote, Argument: "endpoint=http://jaeger:5778"},
			unset:   true,
		},
		{
			name:    "parentbased_jaeger_remote",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.ParentBasedJaegerRemote},
			unset:   true,
		},
		{
			name:    "xray",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.XRaySampler},
			unset:   true,
		},
		{
			name:    "empty type",
			sampler: otelv1alpha1.Sampler{},
			unset:   true,
		},
		{
			// The CRD enum can be bypassed, so an unknown value must not become a rate.
			name:    "unknown type",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.SamplerType("probabilistic")},
			unset:   true,
		},
		{
			name:    "traceidratio without an argument",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.TraceIDRatio},
			unset:   true,
		},
		{
			name:    "traceidratio with a non-numeric argument",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.TraceIDRatio, Argument: "half"},
			unset:   true,
		},
		{
			name:    "traceidratio out of range",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.TraceIDRatio, Argument: "1.5"},
			unset:   true,
		},
		{
			name:    "traceidratio at the bounds",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.TraceIDRatio, Argument: "1"},
			rate:    "1",
		},
		{
			// An argument on a sampler that takes none is simply not used.
			name:    "argument on always_on is ignored",
			sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.AlwaysOn, Argument: "0.5"},
			rate:    "1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := otelv1alpha1.InstrumentationSpec{Sampler: test.sampler}
			translation := Translate(resolvedFor(Java, spec), translatePod())

			rate, rateFound := envValue(t, translation, envDDTraceSampleRate)
			rules, rulesFound := envValue(t, translation, envDDTraceSamplingRules)
			if test.unset {
				assert.False(t, rateFound, "expected no sampling rate, got %q", rate)
				assert.False(t, rulesFound, "expected no sampling rules, got %q", rules)
				return
			}

			// Both are emitted because no single variable covers all four tracers, and
			// they must agree so that whichever one a tracer honours gives the same
			// result.
			require.True(t, rateFound)
			require.True(t, rulesFound)
			assert.Equal(t, test.rate, rate)
			assert.Equal(t, `[{"sample_rate":`+test.rate+`}]`, rules)
		})
	}
}

func TestTranslateSamplerSuppressedByContainerOverride(t *testing.T) {
	// Upstream emits no sampler configuration at all when the container names either
	// half of the OTEL_TRACES_SAMPLER pair, so that a half-configured sampler cannot
	// result. The Datadog pair needs the same joint treatment, because a rate and a rule
	// set that disagree do not merge.
	for _, name := range []string{
		envOTelTracesSampler, envOTelTracesSamplerArg, envDDTraceSampleRate, envDDTraceSamplingRules,
	} {
		t.Run(name, func(t *testing.T) {
			pod := translatePod()
			pod.Spec.Containers[0].Env = []corev1.EnvVar{{Name: name, Value: "whatever"}}

			spec := otelv1alpha1.InstrumentationSpec{
				Sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.TraceIDRatio, Argument: "0.25"},
			}
			translation := Translate(resolvedFor(Java, spec), pod)

			_, rateFound := envValue(t, translation, envDDTraceSampleRate)
			_, rulesFound := envValue(t, translation, envDDTraceSamplingRules)
			assert.False(t, rateFound)
			assert.False(t, rulesFound)
		})
	}
}

func TestTranslateServiceName(t *testing.T) {
	tests := []struct {
		name         string
		annotations  map[string]string
		labels       map[string]string
		useLabels    bool
		attributes   map[string]string
		containerEnv []corev1.EnvVar
		expected     string
		unset        bool
	}{
		{
			// Upstream never overwrites a container's OTEL_SERVICE_NAME, and the SDK
			// gives it precedence over any service.name resource attribute.
			name:         "container OTEL_SERVICE_NAME wins",
			containerEnv: []corev1.EnvVar{{Name: envOTelServiceName, Value: "from-container"}},
			annotations:  map[string]string{resourceAttributeAnnotationPrefix + attrServiceName: "from-annotation"},
			expected:     "from-container",
		},
		{
			name:        "annotation",
			annotations: map[string]string{resourceAttributeAnnotationPrefix + attrServiceName: "from-annotation"},
			expected:    "from-annotation",
		},
		{
			name:        "annotation beats labels",
			annotations: map[string]string{resourceAttributeAnnotationPrefix + attrServiceName: "from-annotation"},
			labels:      map[string]string{labelAppInstance: "from-instance"},
			useLabels:   true,
			expected:    "from-annotation",
		},
		{
			name:      "app.kubernetes.io/instance when labels are enabled",
			labels:    map[string]string{labelAppInstance: "from-instance", labelAppName: "from-name"},
			useLabels: true,
			expected:  "from-instance",
		},
		{
			name:      "app.kubernetes.io/name as the second label",
			labels:    map[string]string{labelAppName: "from-name"},
			useLabels: true,
			expected:  "from-name",
		},
		{
			// The labels are only read when the custom resource opts in.
			name:       "labels ignored without the opt-in",
			labels:     map[string]string{labelAppInstance: "from-instance"},
			useLabels:  false,
			attributes: map[string]string{attrServiceName: "from-cr"},
			expected:   "from-cr",
		},
		{
			name:       "custom resource attribute",
			attributes: map[string]string{attrServiceName: "from-cr"},
			expected:   "from-cr",
		},
		{
			// Nothing named the service, so DD_SERVICE is left to the Datadog
			// owner-reference derivation in the caller rather than guessed here.
			name:  "no source leaves DD_SERVICE unset",
			unset: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := translatePod()
			for key, value := range test.annotations {
				pod.Annotations[key] = value
			}
			for key, value := range test.labels {
				pod.Labels[key] = value
			}
			pod.Spec.Containers[0].Env = test.containerEnv

			spec := otelv1alpha1.InstrumentationSpec{
				Resource: otelv1alpha1.Resource{Attributes: test.attributes},
				Defaults: otelv1alpha1.Defaults{UseLabelsForResourceAttributes: test.useLabels},
			}
			translation := Translate(resolvedFor(Java, spec), pod)

			value, found := envValue(t, translation, "DD_SERVICE")
			if test.unset {
				assert.False(t, found, "expected DD_SERVICE to be left unset, got %q", value)
				return
			}
			require.True(t, found)
			assert.Equal(t, test.expected, value)
		})
	}
}

func TestTranslateServiceNameFromValueFrom(t *testing.T) {
	// An OTEL_SERVICE_NAME sourced from the downward API has to keep working, so the
	// whole variable is copied rather than just its value.
	pod := translatePod()
	source := &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.labels['app']"}}
	pod.Spec.Containers[0].Env = []corev1.EnvVar{{Name: envOTelServiceName, ValueFrom: source}}

	translation := Translate(resolvedFor(Java, otelv1alpha1.InstrumentationSpec{}), pod)
	require.Len(t, translation.Languages, 1)

	var found bool
	for _, env := range translation.Languages[0].EnvVars {
		if env.Name != "DD_SERVICE" {
			continue
		}
		found = true
		assert.Empty(t, env.Value)
		assert.Equal(t, source, env.ValueFrom)
	}
	assert.True(t, found)
}

func TestTranslateServiceVersion(t *testing.T) {
	tests := []struct {
		name        string
		image       string
		annotations map[string]string
		labels      map[string]string
		useLabels   bool
		attributes  map[string]string
		expected    string
		unset       bool
	}{
		{
			name:        "annotation",
			annotations: map[string]string{resourceAttributeAnnotationPrefix + attrServiceVersion: "1.0.0"},
			image:       "registry/app:9.9.9",
			expected:    "1.0.0",
		},
		{
			name:      "label when enabled",
			labels:    map[string]string{labelAppVersion: "2.0.0"},
			useLabels: true,
			image:     "registry/app:9.9.9",
			expected:  "2.0.0",
		},
		{
			name:     "image tag",
			image:    "registry/app:1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "image digest",
			image:    "registry/app@sha256:" + strings.Repeat("a", 64),
			expected: "sha256:" + strings.Repeat("a", 64),
		},
		{
			name:     "image tag and digest",
			image:    "registry/app:1.2.3@sha256:" + strings.Repeat("a", 64),
			expected: "1.2.3@sha256:" + strings.Repeat("a", 64),
		},
		{
			// The custom resource attribute is the last resort, exactly where upstream
			// leaves it: it only survives an image reference with neither tag nor digest.
			name:       "custom resource attribute when the image says nothing",
			image:      "registry/app",
			attributes: map[string]string{attrServiceVersion: "from-cr"},
			expected:   "from-cr",
		},
		{
			name:       "image tag beats the custom resource attribute",
			image:      "registry/app:1.2.3",
			attributes: map[string]string{attrServiceVersion: "from-cr"},
			expected:   "1.2.3",
		},
		{
			name:  "nothing to derive from",
			image: "registry/app",
			unset: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := translatePod()
			pod.Spec.Containers[0].Image = test.image
			for key, value := range test.annotations {
				pod.Annotations[key] = value
			}
			for key, value := range test.labels {
				pod.Labels[key] = value
			}

			spec := otelv1alpha1.InstrumentationSpec{
				Resource: otelv1alpha1.Resource{Attributes: test.attributes},
				Defaults: otelv1alpha1.Defaults{UseLabelsForResourceAttributes: test.useLabels},
			}
			translation := Translate(resolvedFor(Java, spec), pod)

			value, found := envValue(t, translation, "DD_VERSION")
			if test.unset {
				assert.False(t, found, "expected DD_VERSION to be left unset, got %q", value)
				return
			}
			require.True(t, found)
			assert.Equal(t, test.expected, value)
		})
	}
}

func TestTranslateDeploymentEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		attributes  map[string]string
		annotations map[string]string
		expected    string
		unset       bool
	}{
		{
			name:       "semconv 1.27 spelling",
			attributes: map[string]string{attrDeploymentEnvironmentName: "staging"},
			expected:   "staging",
		},
		{
			name:       "legacy spelling",
			attributes: map[string]string{attrDeploymentEnvironment: "staging"},
			expected:   "staging",
		},
		{
			// The Agent's own OTLP mapping prefers the newer key, so this does too.
			name: "the newer key wins",
			attributes: map[string]string{
				attrDeploymentEnvironment:     "old",
				attrDeploymentEnvironmentName: "new",
			},
			expected: "new",
		},
		{
			name:        "annotation overrides the custom resource",
			attributes:  map[string]string{attrDeploymentEnvironmentName: "from-cr"},
			annotations: map[string]string{resourceAttributeAnnotationPrefix + attrDeploymentEnvironmentName: "from-pod"},
			expected:    "from-pod",
		},
		{
			name:  "no environment",
			unset: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := translatePod()
			for key, value := range test.annotations {
				pod.Annotations[key] = value
			}

			spec := otelv1alpha1.InstrumentationSpec{
				Resource: otelv1alpha1.Resource{Attributes: test.attributes},
			}
			translation := Translate(resolvedFor(Java, spec), pod)

			value, found := envValue(t, translation, "DD_ENV")
			if test.unset {
				assert.False(t, found, "expected DD_ENV to be left unset, got %q", value)
				return
			}
			require.True(t, found)
			assert.Equal(t, test.expected, value)
		})
	}
}

func TestTranslateRemainingAttributesBecomeTags(t *testing.T) {
	pod := translatePod()
	pod.Spec.Containers[0].Image = "registry/app:1.2.3"
	pod.Annotations[resourceAttributeAnnotationPrefix+"team"] = "checkout"
	pod.Annotations[resourceAttributeAnnotationPrefix+"tier"] = "from-pod"

	spec := otelv1alpha1.InstrumentationSpec{
		Resource: otelv1alpha1.Resource{Attributes: map[string]string{
			// The four attributes with a dedicated Datadog variable must not be
			// duplicated into DD_TAGS.
			attrServiceName:               "billing",
			attrServiceVersion:            "4.5.6",
			attrDeploymentEnvironmentName: "prod",
			// Overridden by the pod annotation, as upstream orders them.
			"tier":  "from-cr",
			"owner": "payments",
			// An empty value is not a tag.
			"blank": "",
		}},
	}

	translation := Translate(resolvedFor(Java, spec), pod)

	tags, found := envValue(t, translation, envDDTags)
	require.True(t, found)
	// Keys are sorted, so the same custom resource always yields the same pod spec.
	assert.Equal(t, "owner:payments,team:checkout,tier:from-pod", tags)

	service, _ := envValue(t, translation, "DD_SERVICE")
	assert.Equal(t, "billing", service)
	version, _ := envValue(t, translation, "DD_VERSION")
	// The image tag still beats the custom resource attribute.
	assert.Equal(t, "1.2.3", version)
	env, _ := envValue(t, translation, "DD_ENV")
	assert.Equal(t, "prod", env)
}

func TestTranslateNoKubernetesAttributesAndNoVariableReferences(t *testing.T) {
	// The Kubernetes-derived attributes are deliberately dropped: upstream resolves some
	// of them with live API calls, the Agent's tagger already attaches the equivalent
	// tags, and their values are $(VAR) downward-API references whose expansion depends
	// on declaration order.
	pod := translatePod()
	pod.OwnerReferences = []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "app-7d9f8"}}

	spec := otelv1alpha1.InstrumentationSpec{
		Resource: otelv1alpha1.Resource{AddK8sUIDAttributes: true},
		Sampler:  otelv1alpha1.Sampler{Type: otelv1alpha1.AlwaysOn},
	}
	translation := Translate(resolvedFor(Java, spec), pod)
	require.Len(t, translation.Languages, 1)

	for _, env := range translation.Languages[0].EnvVars {
		assert.NotContains(t, env.Value, "$(", "%s must not reference another variable", env.Name)
		assert.NotContains(t, env.Value, "k8s.")
		assert.NotContains(t, env.Value, "service.instance.id")
	}
}

func TestTranslateEnvLayering(t *testing.T) {
	// Upstream's documented precedence is
	// `original container env vars` > `language specific env vars` > `common env vars` >
	// `instrument spec configs' vars`, and every layer is applied with appendIfNotSet.
	// Reproducing it here means ordering the output highest precedence first.
	pod := translatePod()

	spec := otelv1alpha1.InstrumentationSpec{
		Env: []corev1.EnvVar{
			{Name: "SHARED", Value: "from-common"},
			{Name: "COMMON_ONLY", Value: "common"},
		},
		Java: otelv1alpha1.Java{Env: []corev1.EnvVar{
			{Name: "SHARED", Value: "from-java"},
			{Name: "JAVA_ONLY", Value: "java"},
		}},
		Sampler: otelv1alpha1.Sampler{Type: otelv1alpha1.AlwaysOn},
	}

	translation := Translate(resolvedFor(Java, spec), pod)
	require.Len(t, translation.Languages, 1)

	// The per-language block wins over the common one, and the duplicate is gone rather
	// than left for the caller to resolve.
	value, found := envValue(t, translation, "SHARED")
	require.True(t, found)
	assert.Equal(t, "from-java", value)

	names := envNames(translation)
	assert.Equal(t, []string{
		"SHARED", "JAVA_ONLY", "COMMON_ONLY",
		"DD_TRACE_SAMPLE_RATE", "DD_TRACE_SAMPLING_RULES",
	}, names)
}

func TestTranslateLanguageEnvIsPerLanguage(t *testing.T) {
	// Each language reads only its own block, so a nodejs env var must not reach the
	// python configuration.
	spec := otelv1alpha1.InstrumentationSpec{
		Java:   otelv1alpha1.Java{Env: []corev1.EnvVar{{Name: "LANG_ENV", Value: "java"}}},
		NodeJS: otelv1alpha1.NodeJS{Env: []corev1.EnvVar{{Name: "LANG_ENV", Value: "nodejs"}}},
		Python: otelv1alpha1.Python{Env: []corev1.EnvVar{{Name: "LANG_ENV", Value: "python"}}},
		DotNet: otelv1alpha1.DotNet{Env: []corev1.EnvVar{{Name: "LANG_ENV", Value: "dotnet"}}},
	}

	for language, expected := range map[Language]string{
		Java:   "java",
		NodeJS: "nodejs",
		Python: "python",
		DotNet: "dotnet",
	} {
		t.Run(string(language), func(t *testing.T) {
			translation := Translate(resolvedFor(language, spec), translatePod())
			value, found := envValue(t, translation, "LANG_ENV")
			require.True(t, found)
			assert.Equal(t, expected, value)
		})
	}
}

func TestTranslateUserSetVariablesWin(t *testing.T) {
	// A variable the container already declares is the user's. Both the parent package's
	// envVarMutator (dontOverwrite) and upstream's appendIfNotSet behave this way, and
	// the translation refuses to emit it in the first place so that the output says what
	// will actually be added.
	pod := translatePod()
	pod.Spec.Containers[0].Env = []corev1.EnvVar{
		{Name: "DD_SERVICE", Value: "user-service"},
		{Name: "DD_ENV", Value: "user-env"},
		{Name: "DD_VERSION", Value: "user-version"},
		{Name: envDDTags, Value: "user:tags"},
		{Name: envDDPropagationStyle, Value: "datadog"},
		{Name: "SHARED", Value: "user-shared"},
	}

	spec := otelv1alpha1.InstrumentationSpec{
		Resource: otelv1alpha1.Resource{Attributes: map[string]string{
			attrServiceName:               "cr-service",
			attrServiceVersion:            "cr-version",
			attrDeploymentEnvironmentName: "cr-env",
			"team":                        "checkout",
		}},
		Propagators: []otelv1alpha1.Propagator{otelv1alpha1.B3Multi},
		Env:         []corev1.EnvVar{{Name: "SHARED", Value: "cr-shared"}},
		Sampler:     otelv1alpha1.Sampler{Type: otelv1alpha1.AlwaysOn},
	}

	translation := Translate(resolvedFor(Java, spec), pod)

	for _, name := range []string{"DD_SERVICE", "DD_ENV", "DD_VERSION", envDDTags, envDDPropagationStyle, "SHARED"} {
		value, found := envValue(t, translation, name)
		assert.False(t, found, "%s is set by the container and must not be translated, got %q", name, value)
	}

	// What the container did not claim is still translated.
	rate, found := envValue(t, translation, envDDTraceSampleRate)
	require.True(t, found)
	assert.Equal(t, "1", rate)
}

func TestTranslateCollisionOnAnySelectedContainer(t *testing.T) {
	// One environment is applied to every selected container, so a variable claimed by
	// any of them is left alone rather than partially overridden.
	pod := translatePod("app", "sidecar")
	pod.Spec.Containers[1].Env = []corev1.EnvVar{{Name: envDDTags, Value: "user:tags"}}

	spec := otelv1alpha1.InstrumentationSpec{
		Resource: otelv1alpha1.Resource{Attributes: map[string]string{"team": "checkout"}},
	}
	translation := Translate(resolvedFor(Java, spec, "app", "sidecar"), pod)

	_, found := envValue(t, translation, envDDTags)
	assert.False(t, found)
}

func TestTranslateContainerSelection(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "workloads", Name: "app"},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "init-a"}, {Name: "init-b"}},
			Containers:     []corev1.Container{{Name: "first"}, {Name: "second"}},
		},
	}

	tests := []struct {
		name     string
		selected []string
		expected []string
	}{
		{
			// Upstream's default is the first regular container, never all of them and
			// never an init container.
			name:     "no selection means the first regular container",
			selected: nil,
			expected: []string{"first"},
		},
		{
			name:     "explicit selection",
			selected: []string{"second"},
			expected: []string{"second"},
		},
		{
			// Targeted init containers come first, in pod-spec order rather than
			// annotation order.
			name:     "init containers first, in pod-spec order",
			selected: []string{"second", "init-b", "init-a"},
			expected: []string{"init-a", "init-b", "second"},
		},
		{
			name:     "names matching nothing are dropped",
			selected: []string{"second", "typo"},
			expected: []string{"second"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			translation := Translate(resolvedFor(Java, otelv1alpha1.InstrumentationSpec{}, test.selected...), pod)
			require.Len(t, translation.Languages, 1)
			assert.Equal(t, test.expected, translation.Languages[0].Containers)
		})
	}
}

func TestTranslateSelectionMatchingNothingSkipsLanguage(t *testing.T) {
	// Upstream's containersToInstrument comes back empty and its caller skips the
	// language entirely, so there is nothing to inject.
	pod := translatePod()
	translation := Translate(resolvedFor(Java, otelv1alpha1.InstrumentationSpec{}, "typo"), pod)
	assert.True(t, translation.Empty())
}

func TestTranslateNilPod(t *testing.T) {
	translation := Translate(resolvedFor(Java, otelv1alpha1.InstrumentationSpec{}), nil)
	assert.True(t, translation.Empty())
}
