// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"github.com/DataDog/datadog-agent/pkg/serverless/proxy"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProxyProcessor is a InvocationProcessor implementation
type ProxyProcessor struct{}

// OnInvokeStart is the hook triggered when an invocation has started
func (pp *ProxyProcessor) OnInvokeStart(startDetails *proxy.InvocationStartDetails) {
	log.Debug("[proxy] onInvokeStart ------")
	log.Debug("[proxy] Invocation has started at :", startDetails.StartTime)
	log.Debug("[proxy] Invocation invokeHeaders are :", startDetails.InvokeHeaders)
	log.Debug("[proxy] Invocation invokeEvent payload is :", startDetails.InvokeEventPayload)
	log.Debug("[proxy] ---------------------------------------")
}

// OnInvokeEnd is the hook triggered when an invocation has ended
func (pp *ProxyProcessor) OnInvokeEnd(endDetails *proxy.InvocationEndDetails) {
	log.Debug("[proxy] onInvokeEnd ------")
	log.Debug("[proxy] Invocation has finished at :", endDetails.EndTime)
	log.Debug("[proxy] Invocation isError is :", endDetails.IsError)
	log.Debug("[proxy] ---------------------------------------")
}
