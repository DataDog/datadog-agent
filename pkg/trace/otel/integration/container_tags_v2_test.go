// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package integration

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	otlptestutil "github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor/processortest"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type otlpConsumer struct {
	rcv *api.OTLPReceiver
}

var _ consumer.Traces = otlpConsumer{}

func (o otlpConsumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{}
}

func (o otlpConsumer) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	// When ReceiveResourceSpans is used as part of the Datadog exporter, the input will be read-only.
	td.MarkReadOnly()

	for _, rs := range td.ResourceSpans().All() {
		_, err := o.rcv.ReceiveResourceSpans(ctx, rs, http.Header{}, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestContainerTagsV2(t *testing.T) {
	cfg := config.New()
	attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	require.NoError(t, err)
	cfg.OTLPReceiver.AttributesTranslator = attributesTranslator

	// Pretend that container `testid` is found by the tagger through the Kubernetes API
	tagger := otlptestutil.NewTestTaggerClient()
	tagger.TagMap["container_id://testid"] = []string{"container_id:testid", "container_name:testname"}
	cfg.ContainerTags = func(cid string) ([]string, error) {
		return tagger.Tag(types.NewEntityID(types.ContainerID, cid), types.HighCardinality)
	}
	cfg.ContainerIDFromOriginInfo = func(originInfo origindetection.OriginInfo) (string, error) {
		return tagger.GenerateContainerIDFromOriginInfo(originInfo)
	}

	// Enable feature gate
	cfg.Features["enable_otlp_container_tags_v2"] = struct{}{}

	// Set up pipeline with the Infra Attributes Processor + Trace Agent OTLP Receiver
	out := make(chan *api.Payload, 1)
	rcv := api.NewOTLPReceiver(out, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
	factory := infraattributesprocessor.NewFactoryForAgent(tagger, func(_ context.Context) (string, error) {
		return "test-host", nil
	})
	iap, err := factory.CreateTraces(
		t.Context(),
		processortest.NewNopSettings(infraattributesprocessor.Type),
		factory.CreateDefaultConfig(),
		otlpConsumer{rcv: rcv},
	)
	require.NoError(t, err)

	// Span from fake container
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rattr := rs.Resource().Attributes()
	rattr.PutStr("container.id", "testid")
	rattr.PutStr("testresattr", "testval")
	s := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	s.SetName("testspan")
	s.Attributes().PutStr("testspanattr", "testval")

	err = iap.ConsumeTraces(t.Context(), td)
	require.NoError(t, err)

	select {
	case p := <-out:
		require.Len(t, p.TracerPayload.Chunks, 1)
		require.Len(t, p.TracerPayload.Chunks[0].Spans, 1)
		span := p.TracerPayload.Chunks[0].Spans[0]
		containerTags := strings.Split(p.TracerPayload.Tags["_dd.tags.container"], ",")
		sort.Strings(containerTags)
		assert.Equal(t, []string{"container_id:testid", "container_name:testname"}, containerTags, "unexpected container tags")
		assert.Equal(t, "testval", span.Meta["testresattr"], "non-container resource attribute was not passed through")
		assert.Equal(t, "testval", span.Meta["testspanattr"], "span attribute was not passed through")
		assert.Equal(t, "testid", span.Meta["container.id"], "OTel-convention container.id was not passed through")
		for _, key := range []string{"container_id", "container_name"} {
			assert.NotContains(t, span.Meta, key, "pre-mapped container tags are duplicated on span")
		}
	default:
		t.Fatalf("no payload in output channel")
	}
}
