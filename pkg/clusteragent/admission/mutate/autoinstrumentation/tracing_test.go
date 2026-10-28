// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DataDog/dd-policy-engine/go/policies"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/imageresolver"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configwebhook "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/tagsfromlabels"
)

func TestSSITracing(t *testing.T) {
	for _, tt := range []struct {
		name         string
		enabled      bool
		already      bool
		blocked      bool
		skip         string
		outcome      string
		selections   int
		wantInjected bool
	}{
		{name: "selected", enabled: true, outcome: "selected", selections: 3, wantInjected: true},
		{name: "no target", outcome: "no_target", skip: "no_target", selections: 3},
		{name: "already instrumented", enabled: true, already: true, skip: "already_instrumented", selections: 2},
		{name: "blocked", enabled: true, blocked: true, outcome: "blocked", skip: "blocked", selections: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"apm_config.instrumentation.enabled":                           tt.enabled,
				"apm_config.instrumentation.lib_versions":                      map[string]string{"python": "v3"},
				"admission_controller.auto_instrumentation.container_registry": "registry",
				"admission_controller.inject_config.mode":                      "hostip",
			})
			config, err := NewConfig(cfg)
			require.NoError(t, err)
			apm, err := NewTargetMutator(config, mutatecommon.FakeStoreWithDeployment(t, nil), imageresolver.NewNoOpResolver(), nil, nil, mockDDIProvider{})
			require.NoError(t, err)
			if tt.blocked {
				require.NoError(t, apm.SetRemotePolicies([]policies.Policy{podLabelPolicy("sensitive-policy", "app", "sensitive-app", false, nil)}))
			}
			mutators := mutatecommon.NewMutators(
				tagsfromlabels.NewMutator(tagsfromlabels.NewMutatorConfig(cfg), apm),
				configwebhook.NewMutator(configwebhook.NewMutatorConfig(cfg), apm),
				apm,
			)
			webhook, err := NewWebhook(config.Webhook, mutators, NewLabelSelectors(NewLabelSelectorsConfig(cfg)))
			require.NoError(t, err)
			pod := mutatecommon.FakePodWithNamespace("sensitive-pod", "application")
			pod.Labels = map[string]string{"app": "sensitive-app", "tags.datadoghq.com/env": "sensitive-env"}
			if tt.already {
				pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: libraryinjection.InstrumentationVolumeName})
			}
			raw, err := json.Marshal(pod)
			require.NoError(t, err)
			mt := mocktracer.Start()
			defer mt.Stop()
			parent, ctx := tracer.StartSpanFromContext(context.Background(), "parent")
			response := webhook.WebhookFunc()(&admission.Request{Context: ctx, Namespace: pod.Namespace, Object: raw})
			parent.Finish()
			require.True(t, response.Allowed)
			require.Nil(t, response.Result)
			spans := map[string]*mocktracer.Span{}
			var selections []*mocktracer.Span
			for _, span := range mt.FinishedSpans() {
				spans[span.OperationName()] = span
				if span.OperationName() == "cluster_agent.admission.resolve_target" {
					selections = append(selections, span)
				}
				for key, value := range span.Tags() {
					require.NotContains(t, key+fmt.Sprint(value), "sensitive-")
				}
			}
			callback := spans["cluster_agent.admission.mutate_pod"]
			require.NotNil(t, callback)
			for _, stage := range []string{"tags_from_labels", "inject_config", "autoinstrumentation"} {
				span := spans["cluster_agent.admission."+stage]
				require.NotNil(t, span)
				require.Equal(t, callback.SpanID(), span.ParentID(), stage)
			}
			auto := spans["cluster_agent.admission.autoinstrumentation"]
			require.Equal(t, fmt.Sprint(tt.wantInjected), auto.Tag("injected"))
			if tt.skip != "" {
				require.Equal(t, tt.skip, auto.Tag("skip_reason"))
			}
			require.Len(t, selections, tt.selections)
			parents := map[uint64]bool{}
			for _, selection := range selections {
				parents[selection.ParentID()] = true
				if tt.outcome != "" {
					require.Equal(t, tt.outcome, selection.Tag("outcome"))
				}
			}
			for _, stage := range []string{"tags_from_labels", "inject_config"} {
				require.True(t, parents[spans["cluster_agent.admission."+stage].SpanID()])
			}
			injection := spans["cluster_agent.admission.inject_tracers"]
			if tt.wantInjected {
				require.NotNil(t, injection)
				require.Equal(t, auto.SpanID(), injection.ParentID())
			} else {
				require.Nil(t, injection)
			}
		})
	}
}
