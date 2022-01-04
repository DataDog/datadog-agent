// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/api"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProxyProcessor is a InvocationProcessor implementation
type ProxyProcessor struct {
	ExtraTags           *serverlessLog.Tags
	ProcessTrace        func(p *api.Payload)
	MetricChannel       chan []metrics.MetricSample
	DetectLambdaLibrary func() bool
}

// OnInvokeStart is the hook triggered when an invocation has started
func (pp *ProxyProcessor) OnInvokeStart(startDetails *InvocationStartDetails) {
	log.Debug("[proxy] onInvokeStart ------")
	log.Debug("[proxy] Invocation has started at :", startDetails.StartTime)
	log.Debug("[proxy] Invocation invokeHeaders are :", startDetails.InvokeHeaders)
	log.Debug("[proxy] Invocation invokeEvent payload is :", startDetails.InvokeEventPayload)
	log.Debug("[proxy] ---------------------------------------")

	if !pp.DetectLambdaLibrary() {
		startExecutionSpan(startDetails.StartTime)
	}
}

// OnInvokeEnd is the hook triggered when an invocation has ended
func (pp *ProxyProcessor) OnInvokeEnd(endDetails *InvocationEndDetails) {
	log.Debug("[proxy] onInvokeEnd ------")
	log.Debug("[proxy] Invocation has finished at :", endDetails.EndTime)
	log.Debug("[proxy] Invocation isError is :", endDetails.IsError)
	log.Debug("[proxy] ---------------------------------------")

	if !pp.DetectLambdaLibrary() {
		log.Debug("Creating and sending function execution span for invocation")
		endExecutionSpan(pp.ProcessTrace, endDetails.EndTime)
	}

	if endDetails.IsError {
		serverlessMetrics.SendErrorsEnhancedMetric(
			pp.ExtraTags.Tags, endDetails.EndTime, pp.MetricChannel,
		)
	}
}
