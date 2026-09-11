// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/otelinstrumentation"
)

const injectJavaAnnotation = "instrumentation.opentelemetry.io/inject-java"

// newOtelCRWithSpec builds an Instrumentation custom resource with the given spec, for
// the cases newOtelInstrumentationCR's fixed one does not cover.
func newOtelCRWithSpec(namespace, name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": otelv1alpha1.GroupVersion.String(),
		"kind":       "Instrumentation",
		"metadata": map[string]interface{}{
			"namespace": namespace,
			"name":      name,
		},
		"spec": spec,
	}}
}

// newPassthroughMutator builds a mutator whose resolver defaults every language to
// passthrough, which is what the otel_instrumentation_crd_mode setting on "otel" does.
func newPassthroughMutator(t *testing.T, crs ...*unstructured.Unstructured) *TargetMutator {
	t.Helper()
	return newOtelTestMutatorWithResolver(t, func(wmeta workloadmeta.Component) *otelinstrumentation.Resolver {
		return otelinstrumentation.NewResolver(newSyncedOtelStore(t, crs...), NewNamespaceAnnotationGetter(wmeta),
			otelinstrumentation.ModeOTel)
	})
}

// findInitContainer returns the named init container, or nil.
func findInitContainer(pod *corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == name {
			return &pod.Spec.InitContainers[i]
		}
	}
	return nil
}

// TestOtelPassthroughInjectsTheCommunitySDK is the end-to-end shape of passthrough: the
// pod gets upstream's init container, upstream's environment, and none of Datadog's.
func TestOtelPassthroughInjectsTheCommunitySDK(t *testing.T) {
	m := newPassthroughMutator(t, newOtelCRWithSpec(untargetedNamespace, "demo", map[string]interface{}{
		"exporter": map[string]interface{}{"endpoint": "http://collector.default.svc:4318"},
	}))
	pod := newOtelTestPod(untargetedNamespace, nil, map[string]string{injectPythonAnnotation: "true"})

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.True(t, mutated)

	initContainer := findInitContainer(pod, "opentelemetry-auto-instrumentation-python")
	require.NotNil(t, initContainer, "the community SDK init container should have been injected")
	defaultImage, _ := otelinstrumentation.DefaultImage(otelinstrumentation.Python)
	require.Equal(t, defaultImage, initContainer.Image,
		"with no image in the custom resource, the default table stands in for upstream's defaulting webhook")

	env := containerEnv(t, pod, "app")
	require.Contains(t, env, "PYTHONPATH")
	require.Equal(t, "http://collector.default.svc:4318", env["OTEL_EXPORTER_OTLP_ENDPOINT"])

	// The Datadog pipeline must not have run: this pod carries a community SDK, and
	// DD_* variables would configure a tracer that is not there.
	require.NotContains(t, env, "DD_TRACE_PROPAGATION_STYLE")
	require.Empty(t, pod.Spec.Volumes[1:], "only the SDK volume should have been added")
}

// A custom resource can send one language to each mode, and both injections have to land.
func TestOtelPassthroughAndSwapTogether(t *testing.T) {
	// Default mode is swap, and only python names an image, which makes it — and only
	// it — a request for the community SDK.
	m := newOtelTestMutator(t, newOtelCRWithSpec(untargetedNamespace, "demo", map[string]interface{}{
		"python": map[string]interface{}{"image": "registry.acme.example/python-sdk:1.2.3"},
	}))
	pod := newOtelTestPod(untargetedNamespace, nil, map[string]string{
		injectJavaAnnotation:   "true",
		injectPythonAnnotation: "true",
	})

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.True(t, mutated)

	python := findInitContainer(pod, "opentelemetry-auto-instrumentation-python")
	require.NotNil(t, python, "python names a deliberate image, so it goes to passthrough")
	require.Equal(t, "registry.acme.example/python-sdk:1.2.3", python.Image)

	// Java went the Datadog way, which means the Datadog library injection ran too.
	require.Contains(t, containerEnv(t, pod, "app"), "PYTHONPATH")
	require.NotNil(t, findInitContainer(pod, initContainerName("java")))
}

// The webhook can be reinvoked on a pod it already mutated. Appending the SDK a second
// time would load two agents, so the second pass must do nothing at all.
func TestOtelPassthroughIsNotAppliedTwice(t *testing.T) {
	m := newPassthroughMutator(t, newOtelCRWithSpec(untargetedNamespace, "demo", nil))
	pod := newOtelTestPod(untargetedNamespace, nil, map[string]string{injectJavaAnnotation: "true"})

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.True(t, mutated)
	first := containerEnv(t, pod, "app")["JAVA_TOOL_OPTIONS"]
	require.NotEmpty(t, first)

	mutated, err = m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.False(t, mutated, "a reinvocation must not mutate an already injected pod")

	require.Len(t, pod.Spec.InitContainers, 1)
	require.Equal(t, first, containerEnv(t, pod, "app")["JAVA_TOOL_OPTIONS"])
}

// A pod that resolves to nothing injectable must not fall back to the Datadog chain,
// passthrough included: upstream would have left it alone.
func TestOtelPassthroughSelectingNothingDoesNotFallBack(t *testing.T) {
	m := newPassthroughMutator(t, newOtelCRWithSpec(targetedNamespace, "demo", nil))
	pod := newOtelTestPod(targetedNamespace, nil, map[string]string{
		injectPythonAnnotation: "true",
		"instrumentation.opentelemetry.io/python-container-names": "absent",
	})

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)
	require.NoError(t, err)
	require.False(t, mutated)
	require.Empty(t, pod.Spec.InitContainers)
}
