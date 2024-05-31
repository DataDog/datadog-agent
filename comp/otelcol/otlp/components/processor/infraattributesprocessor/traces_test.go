// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
)

// All the data we need to test the Span
type testTrace struct {
	spanName       string
	libraryName    string
	libraryVersion string
}

// All the data we need to define a test
type traceTest struct {
	name     string
	inTraces ptrace.Traces
}

var (
	nameTraces = []testTrace{
		{
			spanName:       "test!",
			libraryName:    "otel",
			libraryVersion: "11",
		},
	}

	standardTraceTests = []traceTest{
		{
			name:     "keepServiceName",
			inTraces: generateTraces(nameTraces),
		},
	}
)

func TestInfraAttributesTraceProcessor(t *testing.T) {
	for _, test := range standardTraceTests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			next := new(consumertest.TracesSink)
			cfg := &Config{}
			fakeTagger := taggerimpl.SetupFakeTagger(t)
			defer fakeTagger.ResetTagger()
			factory := NewFactory(fakeTagger)
			fmp, err := factory.CreateTracesProcessor(
				ctx,
				processortest.NewNopCreateSettings(),
				cfg,
				next,
			)
			require.NotNil(t, fmp)
			require.NoError(t, err)

			caps := fmp.Capabilities()
			require.True(t, caps.MutatesData)

			require.NoError(t, fmp.Start(ctx, nil))

			cErr := fmp.ConsumeTraces(ctx, test.inTraces)
			require.Nil(t, cErr)

			require.NoError(t, fmp.Shutdown(ctx))
		})
	}
}

func generateTraces(traces []testTrace) ptrace.Traces {
	td := ptrace.NewTraces()

	for _, trace := range traces {
		rs := td.ResourceSpans().AppendEmpty()
		//nolint:errcheck
		ils := rs.ScopeSpans().AppendEmpty()
		ils.Scope().SetName(trace.libraryName)
		ils.Scope().SetVersion(trace.libraryVersion)
		span := ils.Spans().AppendEmpty()
		//nolint:errcheck
		span.SetName(trace.spanName)
	}
	return td
}
