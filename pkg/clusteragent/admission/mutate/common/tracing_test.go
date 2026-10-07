// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

func TestMutationStages(t *testing.T) {
	for _, fail := range []bool{false, true} {
		mt := mocktracer.Start()
		root, ctx := tracer.StartSpanFromContext(context.Background(), "request")
		patch, err := MutateWithContext(ctx, []byte(`{"metadata":{"name":"test"}}`), "application", "test", func(ctx context.Context, pod *corev1.Pod, _ string, _ dynamic.Interface) (bool, error) {
			child, _ := tracer.StartSpanFromContext(ctx, "mutation.child")
			child.Finish()
			if fail {
				return false, errors.New("failed mutation")
			}
			pod.Labels = map[string]string{"test": "true"}
			return true, nil
		}, nil)
		root.Finish()
		spans := map[string]*mocktracer.Span{}
		for _, span := range mt.FinishedSpans() {
			spans[span.OperationName()] = span
		}
		mutation := spans["cluster_agent.admission.mutate_pod"]
		require.NotNil(t, mutation)
		assert.Equal(t, mutation.SpanID(), spans["mutation.child"].ParentID())
		for name, span := range spans {
			if name != "request" && name != "mutation.child" {
				assert.Equal(t, spans["request"].SpanID(), span.ParentID())
			}
		}
		if fail {
			require.Error(t, err)
			assert.NotNil(t, mutation.Tag("error.message"))
			assert.Nil(t, spans["cluster_agent.admission.generate_patch"])
		} else {
			require.NoError(t, err)
			require.True(t, json.Valid(patch))
			for _, stage := range []string{"decode_pod", "encode_original_pod", "normalize_pod", "mutate_pod", "encode_mutated_pod", "generate_patch", "encode_patch"} {
				require.NotNil(t, spans["cluster_agent.admission."+stage])
			}
		}
		mt.Stop()
	}
}

func TestTraceStagePanic(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()
	require.PanicsWithValue(t, "test panic", func() {
		_ = TraceStage(context.Background(), "test", func(context.Context) error { panic("test panic") })
	})
	require.Len(t, mt.FinishedSpans(), 1)
	assert.NotNil(t, mt.FinishedSpans()[0].Tag("error.message"))
}
