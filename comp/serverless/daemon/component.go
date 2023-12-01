// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package daemon acts as the communication server between the Serverless runtime and the Datadog agent
package daemon

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	serverlessLog "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
)

// team: serverless-agent

// Component is the component type.
type Component interface {
	GetExecutionContext() *executioncontext.ExecutionContext
	GetExtraTags() *serverlessLog.Tags
	GetFlushStrategy() string
	GetMetricAgent() *metrics.ServerlessMetricAgent
	ShouldFlush(flush.Moment) bool
	Start(time.Time, string, registration.ID, registration.FunctionARN)
	StartLogCollection()
	Stop()
	StoreInvocationTime(time.Time) bool
	TellDaemonRuntimeDone()
	TellDaemonRuntimeStarted()
	TriggerFlush(bool)
	UpdateStrategy()
	WaitForDaemon()
}
