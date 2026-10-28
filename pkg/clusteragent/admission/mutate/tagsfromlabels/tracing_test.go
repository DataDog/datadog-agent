// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagsfromlabels

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestTagsTracing(t *testing.T) {
	for _, tt := range []struct {
		name     string
		labels   bool
		owner    bool
		filtered bool
		fail     bool
	}{
		{name: "pod labels", labels: true},
		{name: "owner lookup", owner: true},
		{name: "owner lookup failure is nonfatal", owner: true, fail: true},
		{name: "filtered", filtered: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pod := mutatecommon.FakePodWithNamespace("sensitive-pod", "sensitive-namespace")
			pod.Labels = map[string]string{"admission.datadoghq.com/enabled": "true"}
			if tt.labels {
				pod.Labels["tags.datadoghq.com/env"] = "sensitive-env"
			}
			if tt.filtered {
				pod.Labels["admission.datadoghq.com/enabled"] = "false"
			}
			client := fake.NewSimpleDynamicClient(runtime.NewScheme())
			if tt.owner {
				ref := metav1.OwnerReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "sensitive-owner"}
				pod.OwnerReferences = []metav1.OwnerReference{ref}
				info, err := getOwnerInfo(ref)
				require.NoError(t, err)
				cache.Cache.Delete(info.buildID(pod.Namespace))
				t.Cleanup(func() { cache.Cache.Delete(info.buildID(pod.Namespace)) })
				if !tt.fail {
					_, err = client.Resource(info.gvr).Namespace(pod.Namespace).Create(context.Background(), &unstructured.Unstructured{Object: map[string]interface{}{
						"apiVersion": "apps/v1", "kind": "Deployment",
						"metadata": map[string]interface{}{"name": ref.Name, "namespace": pod.Namespace, "labels": map[string]interface{}{"tags.datadoghq.com/env": "sensitive-env"}},
					}}, metav1.CreateOptions{})
					require.NoError(t, err)
				}
			}
			filter, err := mutatecommon.NewDefaultFilter(true, nil, nil)
			require.NoError(t, err)
			mutator := NewMutator(&MutatorConfig{ownerCacheTTL: time.Minute}, filter)
			mt := mocktracer.Start()
			defer mt.Stop()
			parent, ctx := tracer.StartSpanFromContext(context.Background(), "parent")
			injected, err := mutator.MutatePodWithContext(ctx, pod, pod.Namespace, client)
			parent.Finish()
			require.NoError(t, err)
			require.Equal(t, !tt.filtered && !tt.fail, injected)
			var tags, lookup *mocktracer.Span
			for _, span := range mt.FinishedSpans() {
				for key, value := range span.Tags() {
					require.NotContains(t, key+fmt.Sprint(value), "sensitive-")
				}
				switch span.OperationName() {
				case "cluster_agent.admission.tags_from_labels":
					tags = span
				case "cluster_agent.admission.owner_lookup":
					lookup = span
				}
			}
			require.NotNil(t, tags)
			require.Nil(t, tags.Tag("error.message"))
			require.Equal(t, fmt.Sprint(injected), tags.Tag("injected"))
			if tt.owner {
				require.NotNil(t, lookup)
				require.Equal(t, tags.SpanID(), lookup.ParentID())
				if tt.fail {
					require.NotNil(t, lookup.Tag("error.message"))
					require.Equal(t, "owner_lookup_failed", tags.Tag("skip_reason"))
				}
			} else {
				require.Nil(t, lookup)
			}
			if injected {
				require.Contains(t, pod.Spec.Containers[0].Env, corev1.EnvVar{Name: "DD_ENV", Value: "sensitive-env"})
			}
		})
	}
}
