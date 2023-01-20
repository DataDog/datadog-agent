// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
*

	ColdStartSpanCreator is needed because the Datadog Agent, when packaged as an extension, can create
	inferred spans (universal instrumentation) or simply pass those spans created by libraries.
	Until all libraries have been updates to utilize Universal Instrumentation, this class
	is necessary to create cold start spans in the span modifier.

*
*/
const (
	service  = "aws.lambda"
	spanName = "aws.lambda.cold_start"
)

var functionName = os.Getenv(functionNameEnvVar)

type ColdStartSpanCreator struct {
	TraceAgent            *ServerlessTraceAgent
	createSpan            sync.Once
	LambdaSpanChan        <-chan *pb.Span
	InitDurationChan      <-chan float64
	syncSpanDurationMutex sync.Mutex
	ColdStartSpanId       uint64
	lambdaSpan            *pb.Span
	initDuration          float64
	StopChan              chan struct{}
}

func (c *ColdStartSpanCreator) Run() {
	go func() {
		for {
			select {
			case traceAgentSpan := <-c.LambdaSpanChan:
				c.handleLambdaSpan(traceAgentSpan)

			case initDuration := <-c.InitDurationChan:
				c.handleInitDuration(initDuration)

			case <-c.StopChan:
				log.Debugf("[ColdStartCreator] - shutting down")
				return
			}
		}
	}()
}

func (c *ColdStartSpanCreator) Stop() {
	log.Debugf("[ColdStartCreator] - sending shutdown msg")
	c.StopChan <- struct{}{}
}

func (c *ColdStartSpanCreator) handleLambdaSpan(traceAgentSpan *pb.Span) {
	if traceAgentSpan.Name == spanName {
		return
	}
	c.syncSpanDurationMutex.Lock()
	defer c.syncSpanDurationMutex.Unlock()

	c.lambdaSpan = traceAgentSpan
	c.createIfReady()
}

func (c *ColdStartSpanCreator) handleInitDuration(initDuration float64) {
	c.syncSpanDurationMutex.Lock()
	defer c.syncSpanDurationMutex.Unlock()
	c.initDuration = initDuration
	c.createIfReady()
}

func (c *ColdStartSpanCreator) createIfReady() {

	if c.initDuration == 0 {
		log.Debug("[ColdStartCreator] No init duration, passing")
		return
	}
	if c.lambdaSpan == nil {
		log.Debug("[ColdStartCreator] No lambda span, passing")
		return
	}
	c.create()
}

func (c *ColdStartSpanCreator) create() {
	// Prevent infinite loop from SpanModifier call
	if c.lambdaSpan.Name == spanName {
		return
	}

	// ColdStartDuration is given in milliseconds
	// APM spans are in nanoseconds
	// millis = nanos * 1e6
	durationNs := c.initDuration * 1e6

	coldStartSpan := &pb.Span{
		Service:  service,
		Name:     spanName,
		Resource: functionName,
		SpanID:   c.ColdStartSpanId,
		TraceID:  c.lambdaSpan.TraceID,
		ParentID: c.lambdaSpan.ParentID,
		Start:    c.lambdaSpan.Start - int64(durationNs),
		Duration: int64(durationNs),
		Type:     "serverless",
	}

	c.createSpan.Do(func() { c.processSpan(coldStartSpan) })
}

func (c *ColdStartSpanCreator) processSpan(coldStartSpan *pb.Span) {
	log.Debugf("[ColdStartCreator] Creating cold start span %v", coldStartSpan)

	traceChunk := &pb.TraceChunk{
		Origin:   "lambda",
		Priority: int32(1),
		Spans:    []*pb.Span{coldStartSpan},
	}

	tracerPayload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{traceChunk},
	}

	c.TraceAgent.ta.Process(&api.Payload{
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
}
