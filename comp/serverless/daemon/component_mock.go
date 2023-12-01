// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package daemon

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
)

// Mock implements mock-specific methods.
type Mock interface {
	Component
	GetInvocationProcessor() invocationlifecycle.InvocationProcessor
	GetRuntimeWg() *sync.WaitGroup
	GetStopped() bool
	GetTellDaemonRuntimeDoneOnce() *sync.Once
	SetInvocationProcessor(m invocationlifecycle.InvocationProcessor)
}
