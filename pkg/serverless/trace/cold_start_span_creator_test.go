// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

func TestColdStartSpanCreatorCreate(t *testing.T) {
	cfg := config.New()
	cfg.GlobalTags = map[string]string{}
	cfg.Endpoints[0].APIKey = "test"
	cfg.Endpoints[0].APIKey = "test"
	ctx, cancel := context.WithCancel(context.Background())
	agnt := agent.NewAgent(ctx, cfg)
	traceAgent := &ServerlessTraceAgent{
		ta: agnt,
	}
	testArn := "arn:aws:lambda:us-east-1:123456789012:function:MY-SUPER-function"
	testColdStartID := "8286a188-ba32-4475-8077-530cd35c09a9"
	ec := &executioncontext.ExecutionContext{}
	ec.SetColdStartDuration(50)
	ec.SetFromInvocation(testArn, testColdStartID)

	coldStartSpanCreator := &ColdStartSpanCreator{
		executionContext: ec,
		traceAgent:       traceAgent,
	}

	defer cancel()

	go traceAgent.Start(true, &LoadConfig{Path: "./testdata/valid.yml"}, nil)

	lambdaSpan := &pb.Span{
		Service:  "aws.lambda",
		Name:     "aws.lambda",
		Start:    time.Now().Unix(),
		TraceID:  random.Random.Uint64(),
		SpanID:   random.Random.Uint64(),
		ParentID: random.Random.Uint64(),
		Duration: 500,
	}
	coldStartSpanCreator.create(lambdaSpan)
	timeout := time.After(2 * time.Second)
	var span *pb.Span
	select {
	case ss := <-traceAgent.ta.TraceWriter.In:
		span = ss.TracerPayload.Chunks[0].Spans[0]
	case <-timeout:
		t.Fatal("timed out")
	}
	assert.Equal(t, "aws.lambda", span.Service)
	assert.Equal(t, "aws.lambda.cold_start", span.Name)

}
