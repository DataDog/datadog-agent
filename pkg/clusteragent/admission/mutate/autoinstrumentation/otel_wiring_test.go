// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/otelinstrumentation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func newOtelTestWorkloadMeta(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Supply(coreconfig.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

// TestTargetMutatorOtelResolverIsInert checks that handing NewTargetMutator a resolver
// changes nothing for a pod that says nothing about OpenTelemetry, and that a nil
// resolver — the shape the feature flag being off produces — is accepted the same way.
// The resolver is consulted now, so this pins down the no-annotation case: it must leave
// the existing precedence chain byte for byte identical. The cases where the annotation
// is present live in otel_target_test.go.
func TestTargetMutatorOtelResolverIsInert(t *testing.T) {
	mutate := func(t *testing.T, resolver *otelinstrumentation.Resolver) *corev1.Pod {
		mockConfig := configmock.NewFromFile(t, "testdata/filter_simple_namespace.yaml")
		mockConfig.SetInTest("admission_controller.auto_instrumentation.container_registry", "registry")
		config, err := NewConfig(mockConfig)
		require.NoError(t, err)

		wmeta := newOtelTestWorkloadMeta(t)
		ns := newTestNamespace("application", nil)
		wmeta.Set(&ns)

		m, err := NewTargetMutator(config, wmeta, imageResolver, nil, resolver, nil)
		require.NoError(t, err)

		pod := mutatecommon.FakePodWithNamespace("foo-service", "application")
		mutated, err := m.MutatePod(pod, pod.Namespace, nil)
		require.NoError(t, err)
		require.True(t, mutated)
		return pod
	}

	withoutResolver := mutate(t, nil)

	resolver := otelinstrumentation.NewResolver(
		otelinstrumentation.NewStore(nil),
		NewNamespaceAnnotationGetter(newOtelTestWorkloadMeta(t)),
		otelinstrumentation.ModeDatadog,
	)
	withResolver := mutate(t, resolver)

	require.Equal(t, withoutResolver, withResolver)
}

func TestNamespaceAnnotationGetter(t *testing.T) {
	wmeta := newOtelTestWorkloadMeta(t)
	wmeta.Set(newTestNamespaceWithAnnotations("annotated", map[string]string{
		"instrumentation.opentelemetry.io/inject-java": "true",
	}))
	wmeta.Set(newTestNamespaceWithAnnotations("bare", nil))

	getter := NewNamespaceAnnotationGetter(wmeta)

	annotations, known := getter.NamespaceAnnotations("annotated")
	require.True(t, known)
	require.Equal(t, map[string]string{"instrumentation.opentelemetry.io/inject-java": "true"}, annotations)

	// A namespace that exists but carries no annotation is known.
	annotations, known = getter.NamespaceAnnotations("bare")
	require.True(t, known)
	require.Empty(t, annotations)

	// A namespace absent from the store is not known, so the caller can tell it apart
	// from "bare" above.
	annotations, known = getter.NamespaceAnnotations("missing")
	require.False(t, known)
	require.Nil(t, annotations)

	// A getter without a store reports every namespace as unknown rather than panicking.
	annotations, known = NewNamespaceAnnotationGetter(nil).NamespaceAnnotations("annotated")
	require.False(t, known)
	require.Nil(t, annotations)
}

func newTestNamespaceWithAnnotations(name string, annotations map[string]string) *workloadmeta.KubernetesMetadata {
	return &workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("", "namespaces", "", name)),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        name,
			Annotations: annotations,
		},
	}
}
