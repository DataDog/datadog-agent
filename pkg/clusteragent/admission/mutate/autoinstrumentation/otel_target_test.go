// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"context"
	"testing"
	"time"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/otelinstrumentation"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// The two namespaces every test below uses. targetedNamespace is the one
// testdata/filter_simple_namespace.yaml aims a target at, so a pod living there would be
// instrumented by SSI on its own; untargetedNamespace is not matched by anything, so
// only an annotation can get a pod there instrumented.
const (
	targetedNamespace   = "application"
	untargetedNamespace = "other"
)

const injectPythonAnnotation = "instrumentation.opentelemetry.io/inject-python"

// newOtelTestMutator builds a TargetMutator whose resolver serves the given custom
// resources. Passing none yields a resolver backed by an empty but synced store, which
// is what a cluster with the CRD installed and no custom resource looks like.
func newOtelTestMutator(t *testing.T, crs ...*unstructured.Unstructured) *TargetMutator {
	t.Helper()
	return newOtelTestMutatorWithResolver(t, func(wmeta workloadmeta.Component) *otelinstrumentation.Resolver {
		return otelinstrumentation.NewResolver(newSyncedOtelStore(t, crs...), NewNamespaceAnnotationGetter(wmeta),
			otelinstrumentation.ModeDatadog)
	})
}

// newOtelTestMutatorWithResolver builds a TargetMutator, letting the caller decide what
// resolver it gets. A resolver of nil is the shape the feature flag being off produces.
func newOtelTestMutatorWithResolver(t *testing.T, resolver func(workloadmeta.Component) *otelinstrumentation.Resolver) *TargetMutator {
	t.Helper()

	mockConfig := configmock.NewFromFile(t, "testdata/filter_simple_namespace.yaml")
	mockConfig.SetInTest("admission_controller.auto_instrumentation.container_registry", "registry")
	config, err := NewConfig(mockConfig)
	require.NoError(t, err)

	wmeta := newOtelTestWorkloadMeta(t)
	for _, name := range []string{targetedNamespace, untargetedNamespace} {
		ns := newTestNamespace(name, nil)
		wmeta.Set(&ns)
	}

	m, err := NewTargetMutator(config, wmeta, imageResolver, nil, resolver(wmeta), nil)
	require.NoError(t, err)
	return m
}

// newSyncedOtelStore returns a Store serving crs, already past its initial cache sync.
func newSyncedOtelStore(t *testing.T, crs ...*unstructured.Unstructured) *otelinstrumentation.Store {
	t.Helper()

	objects := make([]runtime.Object, 0, len(crs))
	for _, cr := range crs {
		objects = append(objects, cr)
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{otelinstrumentation.InstrumentationGVR: "InstrumentationList"},
		objects...,
	)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store := otelinstrumentation.NewStore(client)
	go store.Run(ctx)
	require.Eventually(t, store.HasSynced, 5*time.Second, 10*time.Millisecond,
		"store never finished its initial cache sync")
	return store
}

// newOtelInstrumentationCR builds an Instrumentation custom resource the way a user
// would write it, carrying configuration that has a Datadog equivalent so a translated
// variable proves the resolver and the translation are actually wired together.
func newOtelInstrumentationCR(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": otelv1alpha1.GroupVersion.String(),
		"kind":       "Instrumentation",
		"metadata": map[string]interface{}{
			"namespace": namespace,
			"name":      name,
		},
		"spec": map[string]interface{}{
			"propagators": []interface{}{"tracecontext", "baggage"},
			"sampler": map[string]interface{}{
				"type":     "parentbased_traceidratio",
				"argument": "0.25",
			},
		},
	}}
}

// newOtelTestPod builds a pod with the given labels and annotations and a single
// container named "app".
func newOtelTestPod(namespace string, labels, annotations map[string]string, containerNames ...string) *corev1.Pod {
	if len(containerNames) == 0 {
		containerNames = []string{"app"}
	}
	containers := make([]corev1.Container, 0, len(containerNames))
	for _, name := range containerNames {
		containers = append(containers, corev1.Container{Name: name, Image: "app:1.0"})
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "foo-service",
			Namespace:   namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{Containers: containers},
	}
}

// containerEnv returns the environment of the named container as a map.
func containerEnv(t *testing.T, pod *corev1.Pod, name string) map[string]string {
	t.Helper()

	for _, c := range pod.Spec.Containers {
		if c.Name != name {
			continue
		}
		env := make(map[string]string, len(c.Env))
		for _, e := range c.Env {
			env[e.Name] = e.Value
		}
		return env
	}
	t.Fatalf("pod has no container named %q", name)
	return nil
}

// TestOtelAnnotationOptsInUnlabelledPod is the decision that an OpenTelemetry annotation
// is an opt-in of its own: the pod carries no Datadog label, mutateUnlabelled is off and
// no target matches its namespace, so the annotation is the only reason to instrument it.
func TestOtelAnnotationOptsInUnlabelledPod(t *testing.T) {
	m := newOtelTestMutator(t, newOtelInstrumentationCR(untargetedNamespace, "demo"))
	pod := newOtelTestPod(untargetedNamespace, nil, map[string]string{injectPythonAnnotation: "true"})

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.True(t, mutated, "the OpenTelemetry annotation should have opted this pod in")

	// A variable derived from the custom resource, which only the translation can
	// produce, proves resolution and translation are wired end to end.
	require.Contains(t, containerEnv(t, pod, "app"), "DD_TRACE_PROPAGATION_STYLE")
}

// TestOtelAnnotationIgnoredWithoutResolver checks the feature flag being off leaves the
// annotation completely inert.
func TestOtelAnnotationIgnoredWithoutResolver(t *testing.T) {
	m := newOtelTestMutatorWithResolver(t, func(workloadmeta.Component) *otelinstrumentation.Resolver { return nil })
	pod := newOtelTestPod(untargetedNamespace, nil, map[string]string{injectPythonAnnotation: "true"})

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.False(t, mutated, "no resolver means the annotation must not be honoured")
}

// TestDatadogAnnotationBeatsOtelAnnotation is the decision that Datadog wins when a pod
// carries both kinds of annotation.
func TestDatadogAnnotationBeatsOtelAnnotation(t *testing.T) {
	m := newOtelTestMutator(t, newOtelInstrumentationCR(untargetedNamespace, "demo"))
	pod := newOtelTestPod(untargetedNamespace,
		map[string]string{"admission.datadoghq.com/enabled": "true"},
		map[string]string{
			injectPythonAnnotation:                       "true",
			"admission.datadoghq.com/python-lib.version": "v2",
		})

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.True(t, mutated, "the Datadog annotation should have instrumented this pod")

	// Instrumented on Datadog's terms, so nothing derived from the custom resource.
	require.NotContains(t, containerEnv(t, pod, "app"), "DD_TRACE_PROPAGATION_STYLE")
}

// TestDatadogAnnotationSuppressesOtelOnUnlabelledPod covers the corner the precedence
// rule has to answer for: an unlabelled pod never gets its Datadog library annotation
// applied, so honouring the OpenTelemetry one here would make adding a Datadog
// annotation the thing that triggers an OpenTelemetry-driven injection.
func TestDatadogAnnotationSuppressesOtelOnUnlabelledPod(t *testing.T) {
	m := newOtelTestMutator(t, newOtelInstrumentationCR(untargetedNamespace, "demo"))
	pod := newOtelTestPod(untargetedNamespace, nil, map[string]string{
		injectPythonAnnotation:                       "true",
		"admission.datadoghq.com/python-lib.version": "v2",
	})

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.False(t, mutated, "asking Datadog for a library must suppress the OpenTelemetry path")
}

// TestEnabledFalseLabelBeatsOtelAnnotation is the decision that the explicit opt-out is
// absolute. The pod sits in the targeted namespace, so both SSI and the annotation would
// otherwise instrument it.
func TestEnabledFalseLabelBeatsOtelAnnotation(t *testing.T) {
	m := newOtelTestMutator(t, newOtelInstrumentationCR(targetedNamespace, "demo"))
	pod := newOtelTestPod(targetedNamespace,
		map[string]string{"admission.datadoghq.com/enabled": "false"},
		map[string]string{injectPythonAnnotation: "true"})

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.False(t, mutated, "an explicit opt-out must beat the OpenTelemetry annotation")
}

// TestUnresolvableOtelAnnotationDoesNotFallBack is the fidelity decision, and the
// sharpest of these tests: the pod lives in the targeted namespace, so SSI alone would
// instrument it, but its OpenTelemetry annotation resolves to no custom resource.
// Upstream leaves such a pod alone, so falling back to the target would instrument a pod
// the community Operator would not have touched.
func TestUnresolvableOtelAnnotationDoesNotFallBack(t *testing.T) {
	// The store is synced and empty: the CRD is installed, the custom resource is not.
	m := newOtelTestMutator(t)
	pod := newOtelTestPod(targetedNamespace,
		map[string]string{"admission.datadoghq.com/enabled": "true"},
		map[string]string{injectPythonAnnotation: "true"})

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.False(t, mutated, "an unresolvable annotation must not fall back to the matching target")
}

// TestUnannotatedPodStillMatchesItsTarget guards the other side of the fidelity rule: a
// pod with no OpenTelemetry annotation at all must keep being instrumented by its target
// exactly as before, whether or not a custom resource exists nearby.
func TestUnannotatedPodStillMatchesItsTarget(t *testing.T) {
	m := newOtelTestMutator(t, newOtelInstrumentationCR(targetedNamespace, "demo"))
	pod := newOtelTestPod(targetedNamespace, nil, nil)

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.True(t, mutated, "SSI must keep instrumenting pods that say nothing about OpenTelemetry")
	require.NotContains(t, containerEnv(t, pod, "app"), "DD_TRACE_PROPAGATION_STYLE")
}

// TestOtelInstrumentationOnlyConfiguresSelectedContainers checks the translated
// variables land on the containers the OpenTelemetry contract selected and nowhere else.
// With no selection annotation, upstream instruments the first regular container only.
func TestOtelInstrumentationOnlyConfiguresSelectedContainers(t *testing.T) {
	m := newOtelTestMutator(t, newOtelInstrumentationCR(untargetedNamespace, "demo"))
	pod := newOtelTestPod(untargetedNamespace, nil,
		map[string]string{injectPythonAnnotation: "true"}, "app", "sidecar")

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.True(t, mutated)

	require.Contains(t, containerEnv(t, pod, "app"), "DD_TRACE_PROPAGATION_STYLE")
	require.NotContains(t, containerEnv(t, pod, "sidecar"), "DD_TRACE_PROPAGATION_STYLE",
		"a container the pod did not select must not be configured")
}
