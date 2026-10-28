// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package config

import (
	"context"
	"fmt"
	"testing"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

func TestConfigTracing(t *testing.T) {
	for _, tt := range []struct {
		name     string
		filtered bool
		invalid  bool
	}{
		{name: "injection"},
		{name: "filtered", filtered: true},
		{name: "invalid input", invalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewMockWithOverrides(t, map[string]interface{}{"admission_controller.inject_config.mode": "hostip"})
			filter, err := NewFilter(cfg)
			require.NoError(t, err)
			mutator := NewMutator(NewMutatorConfig(cfg), filter)
			pod := mutatecommon.FakePodWithLabel("admission.datadoghq.com/enabled", "true")
			if tt.filtered {
				pod.Labels["admission.datadoghq.com/enabled"] = "false"
			}
			if tt.invalid {
				pod = nil
			}
			mt := mocktracer.Start()
			defer mt.Stop()
			parent, ctx := tracer.StartSpanFromContext(context.Background(), "parent")
			injected, err := mutator.MutatePodWithContext(ctx, pod, "", nil)
			parent.Finish()
			require.Equal(t, !tt.filtered && !tt.invalid, injected)
			if tt.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			spans := map[string]*mocktracer.Span{}
			for _, span := range mt.FinishedSpans() {
				spans[span.OperationName()] = span
			}
			span := spans["cluster_agent.admission.inject_config"]
			require.NotNil(t, span)
			require.Equal(t, spans["parent"].SpanID(), span.ParentID())
			require.Equal(t, fmt.Sprint(injected), span.Tag("injected"))
			if tt.filtered {
				require.Equal(t, "filtered", span.Tag("skip_reason"))
			}
			if tt.invalid {
				require.NotNil(t, span.Tag("error.message"))
			}
		})
	}
}
