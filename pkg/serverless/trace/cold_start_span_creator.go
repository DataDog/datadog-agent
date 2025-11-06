// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package trace

import (
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
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

type LambdaSpan struct {
	TraceID []byte
	Span    *idx.InternalSpan
}

type ColdStartSpanCreator struct {
	TraceAgent            ServerlessTraceAgent
	createSpan            sync.Once
	LambdaSpanChan        <-chan *LambdaSpan
	LambdaInitMetricChan  <-chan *serverlessLog.LambdaInitMetric
	syncSpanDurationMutex sync.Mutex
	ColdStartSpanId       uint64
	lambdaSpan            *LambdaSpan
	initDuration          float64
	StopChan              chan struct{}
	initStartTime         time.Time
	ColdStartRequestID    string
}

//nolint:revive // TODO(SERV) Fix revive linter
func (c *ColdStartSpanCreator) Run() {
	go func() {
		for {
			select {
			case traceAgentSpan := <-c.LambdaSpanChan:
				c.handleLambdaSpan(traceAgentSpan)
			case initMetric := <-c.LambdaInitMetricChan:
				c.handleInitMetric(initMetric)

			case <-c.StopChan:
				log.Debugf("[ColdStartCreator] - shutting down")
				return
			}
		}
	}()
}

//nolint:revive // TODO(SERV) Fix revive linter
func (c *ColdStartSpanCreator) Stop() {
	log.Debugf("[ColdStartCreator] - sending shutdown msg")
	c.StopChan <- struct{}{}
}

func (c *ColdStartSpanCreator) handleLambdaSpan(traceAgentSpan *LambdaSpan) {
	if traceAgentSpan.Span.Name() == spanName {
		return
	}
	c.syncSpanDurationMutex.Lock()
	defer c.syncSpanDurationMutex.Unlock()

	c.lambdaSpan = traceAgentSpan
	c.createIfReady()
}

func (c *ColdStartSpanCreator) handleInitMetric(initMetric *serverlessLog.LambdaInitMetric) {
	c.syncSpanDurationMutex.Lock()
	defer c.syncSpanDurationMutex.Unlock()
	// Duration and start time come as two separate logs, so we expect this method to be called twice
	if initMetric.InitDurationTelemetry != 0 {
		c.initDuration = initMetric.InitDurationTelemetry
	}
	if !initMetric.InitStartTime.IsZero() {
		c.initStartTime = initMetric.InitStartTime
	}
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
	if c.initStartTime.IsZero() {
		log.Debug("[ColdStartCreator] No init start time, passing")
		return
	}
	c.create()
}

func (c *ColdStartSpanCreator) create() {
	// Prevent infinite loop from SpanModifier call
	if c.lambdaSpan.Span.Name() == spanName {
		return
	}

	// ColdStartDuration is given in milliseconds
	// APM spans are in nanoseconds
	// millis = nanos * 1e6
	durationNs := c.initDuration * 1e6
	var spanStartTime int64
	durationInt := int64(durationNs)
	if (c.initStartTime.UnixNano() + durationInt) < int64(c.lambdaSpan.Span.Start()) {
		spanStartTime = c.initStartTime.UnixNano()
	} else {
		spanStartTime = int64(c.lambdaSpan.Span.Start()) - durationInt
	}

	st := idx.NewStringTable()
	coldStartSpan := idx.NewInternalSpan(st, &idx.Span{
		ServiceRef:  st.Add(service),
		NameRef:     st.Add(spanName),
		ResourceRef: st.Add(functionName),
		SpanID:      c.ColdStartSpanId,
		ParentID:    c.lambdaSpan.Span.ParentID(),
		Start:       uint64(spanStartTime),
		Duration:    uint64(durationNs),
		TypeRef:     st.Add("serverless"),
	})

	coldStartLambdaSpan := &LambdaSpan{
		TraceID: c.lambdaSpan.TraceID,
		Span:    coldStartSpan,
	}

	c.createSpan.Do(func() {
		// An unexpected shutdown will reset this sync.Once counter, so we check whether a cold start has already occurred
		if len(c.ColdStartRequestID) > 0 {
			log.Debugf("[ColdStartCreator] Cold start span already created for request ID %s", c.ColdStartRequestID)
			return
		}
		c.processSpan(coldStartLambdaSpan)
	})
}

func (c *ColdStartSpanCreator) processSpan(coldStartSpan *LambdaSpan) {
	log.Debugf("[ColdStartCreator] Creating cold start span %v", coldStartSpan)

	traceChunk := idx.NewInternalTraceChunk(coldStartSpan.Span.Strings, int32(1), "lambda", nil, []*idx.InternalSpan{coldStartSpan.Span}, false, coldStartSpan.TraceID, 0)

	tracerPayload := &idx.InternalTracerPayload{
		Strings:    coldStartSpan.Span.Strings,
		Chunks:     []*idx.InternalTraceChunk{traceChunk},
		Attributes: make(map[uint32]*idx.AnyValue),
	}

	c.TraceAgent.ProcessV1(&api.PayloadV1{
		Source:        info.NewReceiverStats(true).GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
}
