// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

func TestMutateTracing(t *testing.T) {
	const rawPod = `{"metadata":{"name":"sensitive-pod","namespace":"sensitive-namespace","annotations":{"secret":"sensitive-annotation"}},"spec":{"containers":[{"name":"app","env":[{"name":"SECRET","value":"sensitive-env"}]}]}}`
	for _, tt := range []struct {
		name       string
		raw        string
		inject     bool
		fail       bool
		decodeFail bool
		patch      bool
	}{
		{name: "injection", raw: rawPod, inject: true, patch: true},
		{name: "no mutation", raw: rawPod},
		{name: "normalization only", raw: `{"spec":{"containers":[{"name":"app"}],"volumes":[{"name":"data","emptyDir":{}},{"name":"data","emptyDir":{}}]}}`, patch: true},
		{name: "mutation error", raw: rawPod, fail: true},
		{name: "invalid JSON", raw: `{"spec":`, decodeFail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mt := mocktracer.Start()
			defer mt.Stop()
			parent, ctx := tracer.StartSpanFromContext(context.Background(), "parent")
			called := false
			patch, err := MutateWithContext(ctx, []byte(tt.raw), "sensitive-namespace", "test", func(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
				called = true
				if tt.fail {
					return false, errors.New("sensitive-error: sensitive-env")
				}
				if tt.inject {
					return InjectEnv(pod, corev1.EnvVar{Name: "INJECTED", Value: "sensitive-new-env"}), nil
				}
				return false, nil
			}, nil)
			parent.Finish()
			require.Equal(t, !tt.decodeFail, called)
			if tt.fail || tt.decodeFail {
				require.Error(t, err)
				require.Nil(t, patch)
			} else {
				require.NoError(t, err)
				parsed, err := jsonpatch.DecodePatch(patch)
				require.NoError(t, err)
				require.Equal(t, tt.patch, len(parsed) > 0)
				_, err = parsed.Apply([]byte(tt.raw))
				require.NoError(t, err)
			}

			spans := map[string]*mocktracer.Span{}
			for _, span := range mt.FinishedSpans() {
				spans[span.OperationName()] = span
				for key, value := range span.Tags() {
					require.NotContains(t, key+fmt.Sprint(value), "sensitive-")
				}
			}
			mutation := spans["cluster_agent.admission.mutate"]
			require.NotNil(t, mutation)
			require.Equal(t, spans["parent"].SpanID(), mutation.ParentID())
			if tt.fail || tt.decodeFail {
				require.NotNil(t, mutation.Tag("error.message"))
			}
			if !tt.decodeFail {
				callback := spans["cluster_agent.admission.mutate_pod"]
				require.NotNil(t, callback)
				require.Equal(t, mutation.SpanID(), callback.ParentID())
				require.Equal(t, fmt.Sprint(tt.inject), mutation.Tag("injected"))
			}
			if !tt.fail && !tt.decodeFail {
				require.Equal(t, fmt.Sprint(tt.patch), mutation.Tag("patch_present"))
				require.Equal(t, mutation.SpanID(), spans["cluster_agent.admission.prepare_patch"].ParentID())
			} else {
				require.Nil(t, spans["cluster_agent.admission.prepare_patch"])
			}
		})
	}
}

// Run without mocktracer: it retains spans and would distort allocation results.
// Use this benchmark on the base and feature revisions with the same toolchain
// to compare disabled-tracing overhead.
func BenchmarkMutateTracingDisabled(b *testing.B) {
	for _, containers := range []int{1, 20} {
		for _, inject := range []bool{false, true} {
			b.Run(fmt.Sprintf("containers=%d/inject=%t", containers, inject), func(b *testing.B) {
				var parts []string
				for i := 0; i < containers; i++ {
					parts = append(parts, fmt.Sprintf(`{"name":"app-%d","env":[{"name":"EXISTING","value":"value"}]}`, i))
				}
				raw := []byte(`{"spec":{"containers":[` + strings.Join(parts, ",") + `]}}`)
				mutator := MutatorFunc(func(pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
					if !inject {
						return false, nil
					}
					return InjectEnv(pod, corev1.EnvVar{Name: "INJECTED", Value: "value"}), nil
				})
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := Mutate(raw, "", "test", mutator, nil); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
