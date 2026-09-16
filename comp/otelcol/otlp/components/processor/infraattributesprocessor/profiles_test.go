// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

func TestInfraAttributesProfileProcessor(t *testing.T) {
	cfg := &Config{
		Cardinality:           types.LowCardinality,
		AllowHostnameOverride: true,
	}

	factory := NewFactoryForAgent(testutil.NewTestTaggerClient(), func(_ context.Context) (string, error) { return "test-host", nil })
	next := new(consumertest.ProfilesSink)
	fpp, err := factory.CreateProfiles(context.Background(), processortest.NewNopSettings(Type), cfg, next)
	require.NoError(t, err)
	require.NoError(t, fpp.Start(context.Background(), nil))

	profiles := pprofile.NewProfiles()
	profiles.ResourceProfiles().AppendEmpty().Resource().Attributes().FromRaw(map[string]any{})
	fpp.ConsumeProfiles(context.Background(), profiles)
	require.NoError(t, fpp.Shutdown(context.Background()))

	require.Len(t, next.AllProfiles(), 1)
	require.EqualValues(t, map[string]any{"datadog.host.name": "test-host", "host.arch": runtime.GOARCH}, next.AllProfiles()[0].ResourceProfiles().At(0).Resource().Attributes().AsRaw())

}

// TestInfraAttributesProfileProcessorIgnoresContainerTagPromotion is a
// regression guard: container_tag_promotion only makes sense for traces
// (_dd.tags.container is a trace-agent-specific mechanism), so the profiles
// processor must always behave as "off" even when the option is set to
// duplicate/rename.
func TestInfraAttributesProfileProcessorIgnoresContainerTagPromotion(t *testing.T) {
	for _, mode := range []ContainerTagPromotionMode{ContainerTagPromotionDuplicate, ContainerTagPromotionRename} {
		t.Run(string(mode), func(t *testing.T) {
			cfg := &Config{
				Cardinality:                types.LowCardinality,
				TraceContainerTagPromotion: mode,
			}
			tc := testutil.NewTestTaggerClient()
			tc.TagMap["container_id://test"] = []string{"test_tag:bar"}

			factory := NewFactoryForAgent(tc, func(_ context.Context) (string, error) { return "test-host", nil })
			next := new(consumertest.ProfilesSink)
			fpp, err := factory.CreateProfiles(context.Background(), processortest.NewNopSettings(Type), cfg, next)
			require.NoError(t, err)
			require.NoError(t, fpp.Start(context.Background(), nil))

			profiles := pprofile.NewProfiles()
			profiles.ResourceProfiles().AppendEmpty().Resource().Attributes().FromRaw(map[string]any{"container.id": "test"})
			require.NoError(t, fpp.ConsumeProfiles(context.Background(), profiles))
			require.NoError(t, fpp.Shutdown(context.Background()))

			require.Len(t, next.AllProfiles(), 1)
			out := next.AllProfiles()[0].ResourceProfiles().At(0).Resource().Attributes().AsRaw()
			require.EqualValues(t, map[string]any{
				"container.id": "test",
				"test_tag":     "bar",
				"host.arch":    runtime.GOARCH,
			}, out, "profiles must never gain a datadog.container.tag.* copy, regardless of container_tag_promotion")
		})
	}
}
