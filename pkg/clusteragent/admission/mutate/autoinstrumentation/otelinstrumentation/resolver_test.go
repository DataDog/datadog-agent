// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package otelinstrumentation

import (
	"sort"
	"testing"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// fakeLookup is an in-memory instrumentationLookup that does not need an informer.
type fakeLookup struct {
	// notServing makes every lookup behave like an inert or unsynced store.
	notServing bool
	crs        []*otelv1alpha1.Instrumentation
}

func (f *fakeLookup) Get(namespace, name string) (*otelv1alpha1.Instrumentation, bool) {
	if f.notServing {
		return nil, false
	}
	for _, cr := range f.crs {
		if cr.Namespace == namespace && cr.Name == name {
			return cr, true
		}
	}
	return nil, false
}

func (f *fakeLookup) ListNamespace(namespace string) ([]*otelv1alpha1.Instrumentation, bool) {
	if f.notServing {
		return nil, false
	}
	var out []*otelv1alpha1.Instrumentation
	for _, cr := range f.crs {
		if cr.Namespace == namespace {
			out = append(out, cr)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, true
}

// fakeNamespaces is a NamespaceAnnotationGetter backed by a map.
type fakeNamespaces map[string]map[string]string

func (f fakeNamespaces) NamespaceAnnotations(namespace string) (map[string]string, bool) {
	annotations, ok := f[namespace]
	return annotations, ok
}

func newCR(namespace, name string) *otelv1alpha1.Instrumentation {
	return &otelv1alpha1.Instrumentation{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
		Spec: otelv1alpha1.InstrumentationSpec{
			Exporter: otelv1alpha1.Exporter{Endpoint: "http://collector:4317"},
		},
	}
}

func newPod(annotations map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "app-ns", Name: "my-pod", Annotations: annotations},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}, {Name: "sidecar"}},
		},
	}
}

// newResolver builds a Resolver over fakes. Passing nil namespaces exercises the
// pod-annotations-only degradation.
func newResolver(lookup *fakeLookup, namespaces NamespaceAnnotationGetter) *Resolver {
	return &Resolver{store: lookup, namespaces: namespaces, mode: ModeDatadog, telemetry: newResolverTelemetry()}
}

func injectKey(language Language) string {
	return language.injectAnnotation()
}

func TestResolveNoAnnotation(t *testing.T) {
	lookup := &fakeLookup{crs: []*otelv1alpha1.Instrumentation{newCR("app-ns", "only")}}

	tests := []struct {
		name        string
		annotations map[string]string
	}{
		{
			name:        "no annotations at all",
			annotations: nil,
		},
		{
			name:        "unrelated annotations",
			annotations: map[string]string{"foo": "bar", "admission.datadoghq.com/enabled": "true"},
		},
		{
			name: "every language explicitly disabled",
			annotations: map[string]string{
				injectKey(Java):   "false",
				injectKey(NodeJS): "false",
				injectKey(Python): "false",
				injectKey(DotNet): "false",
			},
		},
		{
			name:        "disabled case insensitively",
			annotations: map[string]string{injectKey(Java): "FALSE"},
		},
		{
			name:        "empty annotation value",
			annotations: map[string]string{injectKey(Java): ""},
		},
		{
			// go is an explicit non-goal, so a go-only pod is invisible here and must
			// fall through to the existing SSI chain rather than be refused.
			name:        "an out-of-scope language only",
			annotations: map[string]string{"instrumentation.opentelemetry.io/inject-go": "true"},
		},
		{
			// Upstream validates container-names only after deciding at least one
			// language is requested, so a malformed value alone changes nothing.
			name:        "malformed container names without any inject annotation",
			annotations: map[string]string{commonContainerNamesAnnotation: "bad_name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := newResolver(lookup, nil).Resolve(newPod(tt.annotations), "app-ns")

			assert.Equal(t, OutcomeNoAnnotation, result.Outcome,
				"must fall through to the existing SSI chain")
			assert.Empty(t, result.Languages)
			assert.Empty(t, result.Reason)
		})
	}
}

func TestResolveSoleInNamespace(t *testing.T) {
	tests := []struct {
		name        string
		crs         []*otelv1alpha1.Instrumentation
		wantOutcome Outcome
		wantReason  Reason
		wantCRName  string
	}{
		{
			name:        "zero custom resources is unresolvable",
			crs:         nil,
			wantOutcome: OutcomeUnresolvable,
			wantReason:  ReasonNoInstrumentationInNamespace,
		},
		{
			name:        "exactly one is resolved",
			crs:         []*otelv1alpha1.Instrumentation{newCR("app-ns", "only")},
			wantOutcome: OutcomeResolved,
			wantCRName:  "only",
		},
		{
			// There is no default custom resource name upstream, so two candidates is
			// ambiguous rather than a tie to break.
			name:        "several is unresolvable",
			crs:         []*otelv1alpha1.Instrumentation{newCR("app-ns", "a"), newCR("app-ns", "b")},
			wantOutcome: OutcomeUnresolvable,
			wantReason:  ReasonMultipleInstrumentations,
		},
		{
			name:        "custom resources in other namespaces do not count",
			crs:         []*otelv1alpha1.Instrumentation{newCR("app-ns", "only"), newCR("other-ns", "b")},
			wantOutcome: OutcomeResolved,
			wantCRName:  "only",
		},
		{
			name:        "only other namespaces populated is unresolvable",
			crs:         []*otelv1alpha1.Instrumentation{newCR("other-ns", "b")},
			wantOutcome: OutcomeUnresolvable,
			wantReason:  ReasonNoInstrumentationInNamespace,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := newResolver(&fakeLookup{crs: tt.crs}, nil)
			pod := newPod(map[string]string{injectKey(Java): "true"})

			result := resolver.Resolve(pod, "app-ns")

			require.Equal(t, tt.wantOutcome, result.Outcome)
			if tt.wantOutcome != OutcomeResolved {
				assert.Equal(t, tt.wantReason, result.Reason)
				assert.Equal(t, Java, result.ReasonLanguage)
				assert.Empty(t, result.Languages)
				return
			}
			require.Len(t, result.Languages, 1)
			assert.Equal(t, Java, result.Languages[0].Language)
			assert.Equal(t, tt.wantCRName, result.Languages[0].Instrumentation.Name)
		})
	}
}

func TestResolveNamedInstrumentation(t *testing.T) {
	lookup := &fakeLookup{crs: []*otelv1alpha1.Instrumentation{
		newCR("app-ns", "local"),
		newCR("other-ns", "remote"),
	}}

	tests := []struct {
		name          string
		value         string
		wantOutcome   Outcome
		wantReason    Reason
		wantNamespace string
		wantCRName    string
	}{
		{
			name:          "bare name resolves in the pod namespace",
			value:         "local",
			wantOutcome:   OutcomeResolved,
			wantNamespace: "app-ns",
			wantCRName:    "local",
		},
		{
			name:          "qualified name resolves across namespaces",
			value:         "other-ns/remote",
			wantOutcome:   OutcomeResolved,
			wantNamespace: "other-ns",
			wantCRName:    "remote",
		},
		{
			name:        "missing name is unresolvable",
			value:       "nope",
			wantOutcome: OutcomeUnresolvable,
			wantReason:  ReasonInstrumentationNotFound,
		},
		{
			name:        "name that exists only in another namespace is unresolvable",
			value:       "remote",
			wantOutcome: OutcomeUnresolvable,
			wantReason:  ReasonInstrumentationNotFound,
		},
		{
			name:        "qualified name with a missing namespace is unresolvable",
			value:       "nope-ns/local",
			wantOutcome: OutcomeUnresolvable,
			wantReason:  ReasonInstrumentationNotFound,
		},
		{
			name:        "unrecognised value is treated as a name and misses",
			value:       "yes",
			wantOutcome: OutcomeUnresolvable,
			wantReason:  ReasonInstrumentationNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := newResolver(lookup, nil)
			pod := newPod(map[string]string{injectKey(Python): tt.value})

			result := resolver.Resolve(pod, "app-ns")

			require.Equal(t, tt.wantOutcome, result.Outcome)
			if tt.wantOutcome != OutcomeResolved {
				assert.Equal(t, tt.wantReason, result.Reason)
				assert.Equal(t, Python, result.ReasonLanguage)
				return
			}
			require.Len(t, result.Languages, 1)
			assert.Equal(t, tt.wantNamespace, result.Languages[0].Instrumentation.Namespace)
			assert.Equal(t, tt.wantCRName, result.Languages[0].Instrumentation.Name)
		})
	}
}

func TestResolveStoreUnavailable(t *testing.T) {
	// An inert or unsynced store must refuse rather than pretend nothing was requested,
	// otherwise a pod would silently fall through to the Datadog chain.
	resolver := newResolver(&fakeLookup{notServing: true}, nil)

	result := resolver.Resolve(newPod(map[string]string{injectKey(Java): "true"}), "app-ns")

	assert.Equal(t, OutcomeUnresolvable, result.Outcome)
	assert.Equal(t, ReasonStoreUnavailable, result.Reason)
	assert.Equal(t, Java, result.ReasonLanguage)
}

func TestResolvePodNamespacePrecedence(t *testing.T) {
	lookup := &fakeLookup{crs: []*otelv1alpha1.Instrumentation{
		newCR("app-ns", "pod-cr"),
		newCR("app-ns", "ns-cr"),
	}}

	tests := []struct {
		name        string
		podValue    string
		nsValue     string
		wantOutcome Outcome
		wantCRName  string
		wantReason  Reason
		rationale   string
	}{
		{
			name:        "namespace applies when the pod is silent",
			nsValue:     "ns-cr",
			wantOutcome: OutcomeResolved,
			wantCRName:  "ns-cr",
			rationale:   "a namespace annotation covers all its pods",
		},
		{
			name:        "pod applies when the namespace is silent",
			podValue:    "pod-cr",
			wantOutcome: OutcomeResolved,
			wantCRName:  "pod-cr",
		},
		{
			name:        "pod false beats namespace true",
			podValue:    "false",
			nsValue:     "true",
			wantOutcome: OutcomeNoAnnotation,
			rationale:   "an opt-out on the pod is always honoured",
		},
		{
			name:        "pod false beats a namespace named CR",
			podValue:    "false",
			nsValue:     "ns-cr",
			wantOutcome: OutcomeNoAnnotation,
			rationale:   "an opt-out on the pod is always honoured",
		},
		{
			name:        "pod named CR beats a namespace named CR",
			podValue:    "pod-cr",
			nsValue:     "ns-cr",
			wantOutcome: OutcomeResolved,
			wantCRName:  "pod-cr",
			rationale:   "an explicit pod choice is final",
		},
		{
			name:        "pod true beats namespace false",
			podValue:    "true",
			nsValue:     "false",
			wantOutcome: OutcomeUnresolvable,
			wantReason:  ReasonMultipleInstrumentations,
			rationale:   "the pod re-enables, so 'true' applies and both CRs are candidates",
		},
		{
			// The load-bearing counter-intuitive case.
			name:        "namespace named CR beats pod true",
			podValue:    "true",
			nsValue:     "ns-cr",
			wantOutcome: OutcomeResolved,
			wantCRName:  "ns-cr",
			rationale:   "pod 'true' only opts in and delegates the choice of CR",
		},
		{
			name:        "namespace named CR beats pod TRUE case-insensitively",
			podValue:    "TRUE",
			nsValue:     "ns-cr",
			wantOutcome: OutcomeResolved,
			wantCRName:  "ns-cr",
		},
		{
			name:        "both true falls back to the exactly-one rule",
			podValue:    "true",
			nsValue:     "true",
			wantOutcome: OutcomeUnresolvable,
			wantReason:  ReasonMultipleInstrumentations,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podAnnotations := map[string]string{}
			if tt.podValue != "" {
				podAnnotations[injectKey(Java)] = tt.podValue
			}
			nsAnnotations := map[string]string{}
			if tt.nsValue != "" {
				nsAnnotations[injectKey(Java)] = tt.nsValue
			}

			resolver := newResolver(lookup, fakeNamespaces{"app-ns": nsAnnotations})
			result := resolver.Resolve(newPod(podAnnotations), "app-ns")

			require.Equal(t, tt.wantOutcome, result.Outcome, tt.rationale)
			switch tt.wantOutcome {
			case OutcomeResolved:
				require.Len(t, result.Languages, 1)
				assert.Equal(t, tt.wantCRName, result.Languages[0].Instrumentation.Name, tt.rationale)
			case OutcomeUnresolvable:
				assert.Equal(t, tt.wantReason, result.Reason, tt.rationale)
			}
		})
	}
}

func TestResolveIgnoresNamespaceAnnotationsWithoutAGetter(t *testing.T) {
	lookup := &fakeLookup{crs: []*otelv1alpha1.Instrumentation{newCR("app-ns", "ns-cr")}}

	// With no getter the namespace looks unannotated, so a namespace-only opt-in is
	// invisible and the pod falls through.
	result := newResolver(lookup, nil).Resolve(newPod(nil), "app-ns")
	assert.Equal(t, OutcomeNoAnnotation, result.Outcome)

	// And a getter that does not know the namespace behaves the same way.
	result = newResolver(lookup, fakeNamespaces{}).Resolve(newPod(nil), "app-ns")
	assert.Equal(t, OutcomeNoAnnotation, result.Outcome)

	// Supplying the annotations turns the same pod into a resolved one.
	namespaces := fakeNamespaces{"app-ns": {injectKey(Java): "true"}}
	result = newResolver(lookup, namespaces).Resolve(newPod(nil), "app-ns")
	require.Equal(t, OutcomeResolved, result.Outcome)
	assert.Equal(t, "ns-cr", result.Languages[0].Instrumentation.Name)
}

func TestResolveMultipleLanguages(t *testing.T) {
	lookup := &fakeLookup{crs: []*otelv1alpha1.Instrumentation{
		newCR("app-ns", "shared"),
		newCR("app-ns", "java-specific"),
	}}

	t.Run("languages are reported in upstream evaluation order", func(t *testing.T) {
		pod := newPod(map[string]string{
			injectKey(DotNet): "shared",
			injectKey(Java):   "shared",
			injectKey(Python): "shared",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		require.Equal(t, OutcomeResolved, result.Outcome)
		require.Len(t, result.Languages, 3)
		assert.Equal(t, Java, result.Languages[0].Language)
		assert.Equal(t, Python, result.Languages[1].Language)
		assert.Equal(t, DotNet, result.Languages[2].Language)
	})

	t.Run("each language keeps its own custom resource", func(t *testing.T) {
		pod := newPod(map[string]string{
			injectKey(Java):   "java-specific",
			injectKey(Python): "shared",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		require.Equal(t, OutcomeResolved, result.Outcome)
		require.Len(t, result.Languages, 2)
		assert.Equal(t, "java-specific", result.Languages[0].Instrumentation.Name)
		assert.Equal(t, "shared", result.Languages[1].Instrumentation.Name)
	})

	t.Run("a disabled language is dropped and the others still resolve", func(t *testing.T) {
		pod := newPod(map[string]string{
			injectKey(Java):   "false",
			injectKey(Python): "shared",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		require.Equal(t, OutcomeResolved, result.Outcome)
		require.Len(t, result.Languages, 1)
		assert.Equal(t, Python, result.Languages[0].Language)
	})

	t.Run("one unresolvable language poisons the whole pod", func(t *testing.T) {
		// Upstream aborts Mutate on the first failing language, leaving the pod
		// completely uninstrumented even though python would have resolved.
		pod := newPod(map[string]string{
			injectKey(Java):   "nope",
			injectKey(Python): "shared",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		assert.Equal(t, OutcomeUnresolvable, result.Outcome)
		assert.Equal(t, ReasonInstrumentationNotFound, result.Reason)
		assert.Equal(t, Java, result.ReasonLanguage)
		assert.Empty(t, result.Languages)
	})

	t.Run("the first failing language in upstream order is reported", func(t *testing.T) {
		pod := newPod(map[string]string{
			injectKey(Python): "nope-python",
			injectKey(DotNet): "nope-dotnet",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		assert.Equal(t, OutcomeUnresolvable, result.Outcome)
		assert.Equal(t, Python, result.ReasonLanguage, "python precedes dotnet upstream")
	})
}

func TestResolveContainerSelection(t *testing.T) {
	lookup := &fakeLookup{crs: []*otelv1alpha1.Instrumentation{newCR("app-ns", "shared")}}

	t.Run("no selection leaves containers empty", func(t *testing.T) {
		pod := newPod(map[string]string{injectKey(Java): "shared"})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		require.Equal(t, OutcomeResolved, result.Outcome)
		assert.Empty(t, result.Languages[0].Containers,
			"defaulting to the first container belongs to the translation pass")
	})

	t.Run("common selection applies to every requested language", func(t *testing.T) {
		pod := newPod(map[string]string{
			injectKey(Java):                "shared",
			injectKey(Python):              "shared",
			commonContainerNamesAnnotation: "app,sidecar",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		require.Equal(t, OutcomeResolved, result.Outcome)
		require.Len(t, result.Languages, 2)
		assert.Equal(t, []string{"app", "sidecar"}, result.Languages[0].Containers)
		assert.Equal(t, []string{"app", "sidecar"}, result.Languages[1].Containers)
	})

	t.Run("per-language selection is appended to the common one", func(t *testing.T) {
		// Upstream appends rather than replacing, even though its multi-instrumentation
		// doc claims container-names is unused in this mode.
		pod := newPod(map[string]string{
			injectKey(Java):                 "shared",
			commonContainerNamesAnnotation:  "app",
			Java.containerNamesAnnotation(): "extra",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		require.Equal(t, OutcomeResolved, result.Outcome)
		assert.Equal(t, []string{"app", "extra"}, result.Languages[0].Containers)
	})

	t.Run("per-language selection alone is used", func(t *testing.T) {
		pod := newPod(map[string]string{
			injectKey(Java):                 "shared",
			Java.containerNamesAnnotation(): "only-java",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		require.Equal(t, OutcomeResolved, result.Outcome)
		assert.Equal(t, []string{"only-java"}, result.Languages[0].Containers)
	})

	t.Run("another language's selection does not leak", func(t *testing.T) {
		pod := newPod(map[string]string{
			injectKey(Java):                   "shared",
			injectKey(Python):                 "shared",
			Python.containerNamesAnnotation(): "py-only",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		require.Equal(t, OutcomeResolved, result.Outcome)
		require.Len(t, result.Languages, 2)
		assert.Empty(t, result.Languages[0].Containers, "java made no selection")
		assert.Equal(t, []string{"py-only"}, result.Languages[1].Containers)
	})

	t.Run("namespace container selection applies", func(t *testing.T) {
		pod := newPod(map[string]string{injectKey(Java): "shared"})
		namespaces := fakeNamespaces{"app-ns": {commonContainerNamesAnnotation: "from-ns"}}

		result := newResolver(lookup, namespaces).Resolve(pod, "app-ns")

		require.Equal(t, OutcomeResolved, result.Outcome)
		assert.Equal(t, []string{"from-ns"}, result.Languages[0].Containers)
	})

	t.Run("invalid common selection makes the pod unresolvable", func(t *testing.T) {
		pod := newPod(map[string]string{
			injectKey(Java):                "shared",
			commonContainerNamesAnnotation: "my_app",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		assert.Equal(t, OutcomeUnresolvable, result.Outcome)
		assert.Equal(t, ReasonInvalidContainerNames, result.Reason)
		assert.Empty(t, result.ReasonLanguage, "the common annotation is not language specific")
	})

	t.Run("invalid per-language selection names the language", func(t *testing.T) {
		pod := newPod(map[string]string{
			injectKey(Java):                 "shared",
			Java.containerNamesAnnotation(): "my.app",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		assert.Equal(t, OutcomeUnresolvable, result.Outcome)
		assert.Equal(t, ReasonInvalidContainerNames, result.Reason)
		assert.Equal(t, Java, result.ReasonLanguage)
	})

	t.Run("an unrequested language's invalid selection is ignored", func(t *testing.T) {
		// Only requested languages have their per-language annotation read, so a stale
		// annotation for a language nobody asked for cannot break the pod.
		pod := newPod(map[string]string{
			injectKey(Java):                   "shared",
			Python.containerNamesAnnotation(): "my_app",
		})

		result := newResolver(lookup, nil).Resolve(pod, "app-ns")

		assert.Equal(t, OutcomeResolved, result.Outcome)
	})
}

func TestResolveCarriesTheModeDecision(t *testing.T) {
	// The configured mode reaches every language of the result, and a custom resource
	// naming its own SDK image overrides it for that language alone. The decision itself
	// is covered by mode_test.go; this checks it is actually carried through resolution,
	// which is what the translation pass branches on.
	pinned := newCR("app-ns", "pinned")
	pinned.Spec.Python.Image = "registry.example.com/our-python-sdk:1"

	resolver := newResolver(&fakeLookup{crs: []*otelv1alpha1.Instrumentation{pinned}}, nil)
	pod := newPod(map[string]string{
		injectKey(Java):   "pinned",
		injectKey(Python): "pinned",
	})

	result := resolver.Resolve(pod, "app-ns")

	require.Equal(t, OutcomeResolved, result.Outcome)
	require.Len(t, result.Languages, 2)

	byLanguage := map[Language]LanguageRequest{}
	for _, request := range result.Languages {
		byLanguage[request.Language] = request
	}

	assert.Equal(t, ModeDatadog, byLanguage[Java].Mode)
	assert.Equal(t, ModeSourceNoImage, byLanguage[Java].ModeSource)
	assert.Equal(t, ModeOTel, byLanguage[Python].Mode)
	assert.Equal(t, ModeSourceUnknownImage, byLanguage[Python].ModeSource)
}

func TestResolveInPassthroughMode(t *testing.T) {
	lookup := &fakeLookup{crs: []*otelv1alpha1.Instrumentation{newCR("app-ns", "shared")}}
	resolver := &Resolver{store: lookup, mode: ModeOTel, telemetry: newResolverTelemetry()}

	result := resolver.Resolve(newPod(map[string]string{injectKey(Java): "shared"}), "app-ns")

	require.Equal(t, OutcomeResolved, result.Outcome)
	require.Len(t, result.Languages, 1)
	assert.Equal(t, ModeOTel, result.Languages[0].Mode)
}

func TestResolveDoesNotMutateThePod(t *testing.T) {
	lookup := &fakeLookup{crs: []*otelv1alpha1.Instrumentation{newCR("app-ns", "shared")}}
	pod := newPod(map[string]string{injectKey(Java): "shared"})

	result := newResolver(lookup, nil).Resolve(pod, "app-ns")

	require.Equal(t, OutcomeResolved, result.Outcome)
	assert.Len(t, pod.Annotations, 1, "resolution must be read-only")
	assert.Len(t, pod.Spec.Containers, 2)
}
