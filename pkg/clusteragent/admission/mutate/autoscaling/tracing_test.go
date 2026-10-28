// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
)

type tracingPodPatcher struct {
	workload.PodPatcher
	fail bool
}

func (p tracingPodPatcher) ApplyRecommendations(*corev1.Pod) (bool, error) {
	if p.fail {
		return false, errors.New("sensitive-error")
	}
	return false, nil
}

func TestAutoscalingTracing(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail=%t", fail), func(t *testing.T) {
			cfg := config.NewMock(t)
			webhook := NewWebhook(tracingPodPatcher{fail: fail}, cfg)
			mt := mocktracer.Start()
			defer mt.Stop()
			parent, ctx := tracer.StartSpanFromContext(context.Background(), "parent")
			response := webhook.WebhookFunc()(&admission.Request{
				Context: ctx,
				Object:  []byte(`{"metadata":{"name":"sensitive-pod"},"spec":{"containers":[{"name":"app"}]}}`),
			})
			parent.Finish()
			require.True(t, response.Allowed, "mutation failures must remain fail-open")
			if fail {
				require.NotNil(t, response.Result)
				require.Contains(t, response.Result.Message, "sensitive-error", "original response must be preserved")
			} else {
				require.Nil(t, response.Result)
			}
			var callback, mutation *mocktracer.Span
			for _, span := range mt.FinishedSpans() {
				require.NotContains(t, fmt.Sprint(span.Tags()), "sensitive-")
				if span.OperationName() == "cluster_agent.admission.mutate_pod" {
					callback = span
				}
				if span.OperationName() == "cluster_agent.admission.mutate" {
					mutation = span
				}
			}
			require.NotNil(t, mutation)
			require.NotNil(t, callback)
			require.Equal(t, mutation.SpanID(), callback.ParentID())
			require.Equal(t, "autoscaling", callback.Tag("resource.name"))
			require.Equal(t, "false", callback.Tag("injected"))
			if fail {
				require.NotNil(t, callback.Tag("error.message"))
			}
		})
	}
}
