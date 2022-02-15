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

// LifecycleProcessor is a InvocationProcessor implementation
type LifecycleProcessor struct {
	ExtraTags           *serverlessLog.Tags
	ProcessTrace        func(p *api.Payload)
	MetricChannel       chan []metrics.MetricSample
	DetectLambdaLibrary func() bool
}

// OnInvokeStart is the hook triggered when an invocation has started
func (lp *LifecycleProcessor) OnInvokeStart(startDetails *InvocationStartDetails) {
	log.Debug("[lifecycle] onInvokeStart ------")
	log.Debug("[lifecycle] Invocation has started at :", startDetails.StartTime)
	log.Debug("[lifecycle] Invocation invokeEvent payload is :", startDetails.InvokeEventRawPayload)
	log.Debug("[lifecycle] ---------------------------------------")

	if !lp.DetectLambdaLibrary() {
		startExecutionSpan(startDetails.StartTime, startDetails.InvokeEventRawPayload)
	}
}

// OnInvokeEnd is the hook triggered when an invocation has ended
func (lp *LifecycleProcessor) OnInvokeEnd(endDetails *InvocationEndDetails) {
	log.Debug("[lifecycle] onInvokeEnd ------")
	log.Debug("[lifecycle] Invocation has finished at :", endDetails.EndTime)
	log.Debug("[lifecycle] Invocation isError is :", endDetails.IsError)
	log.Debug("[lifecycle] ---------------------------------------")

	if !lp.DetectLambdaLibrary() {
		log.Debug("Creating and sending function execution span for invocation")
		endExecutionSpan(lp.ProcessTrace, endDetails.RequestID, endDetails.EndTime, endDetails.IsError)
	}

	if endDetails.IsError {
		serverlessMetrics.SendErrorsEnhancedMetric(
			lp.ExtraTags.Tags, endDetails.EndTime, lp.MetricChannel,
		)
	}
}
