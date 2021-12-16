// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/assert"
)

func TestServerlessServiceRewrite(t *testing.T) {
	cfg := config.New()
	cfg.GlobalTags = map[string]string{
		"service": "myTestService",
	}
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := agent.NewAgent(ctx, cfg)
	spanProcessor := &spanProcessor{
		tags: cfg.GlobalTags,
	}
	agnt.ProcessSpan = spanProcessor.Process
	defer cancel()

	tp := testutil.TracerPayloadWithChunk(testutil.RandomTraceChunk(1, 1))
	tp.Chunks[0].Spans[0].Service = "aws.lambda"
	go agnt.Process(&api.Payload{
		TracerPayload: tp,
		Source:        agnt.Receiver.Stats.GetTagStats(info.Tags{}),
	})
	timeout := time.After(2 * time.Second)
	var span *pb.Span
	select {
	case ss := <-agnt.TraceWriter.In:
		span = ss.TracerPayload.Chunks[0].Spans[0]
	case <-timeout:
		t.Fatal("timed out")
	}
	assert.Equal(t, "myTestService", span.Service)
}
