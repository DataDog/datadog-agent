// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"

// InvocationProcessor is the interface to implement to receive invocation lifecycle hooks
type InvocationProcessor interface {
	// OnInvokeStart is the hook triggered when an invocation has started
	OnInvokeStart(startDetails *InvocationStartDetails, ctx *serverlessLog.ExecutionContext)
	// OnInvokeEnd is the hook triggered when an invocation has ended
	OnInvokeEnd(endDetails *InvocationEndDetails)
}
