// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build serverlessexperimental

package proxy

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type invocationProcessor interface {
	process(details *invocationDetails)
}

type proxyProcessor struct{}

func (pp *proxyProcessor) process(invocationDetails *invocationDetails) {
	// TODO here is the part where trace/spans will be created as all information is now available
	log.Debug("[proxy] Invocation ready to be processed ------")
	log.Debug("[proxy] Invocation has started at :", invocationDetails.startTime)
	log.Debug("[proxy] Invocation has finished at :", invocationDetails.endTime)
	log.Debug("[proxy] Invocation invokeHeaders is :", invocationDetails.invokeHeaders)
	log.Debug("[proxy] Invocation invokeEvent payload is :", invocationDetails.invokeEventPayload)
	log.Debug("[proxy] Invocation is in error? :", invocationDetails.isError)
	log.Debug("[proxy] ---------------------------------------")
}
