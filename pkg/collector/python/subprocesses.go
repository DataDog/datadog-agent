// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build python

package python

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type subprocessContext struct {
	mux    sync.RWMutex
	Ctx    context.Context
	Cancel context.CancelFunc
}

var subprocCtx *subprocessContext
var once sync.Once

// GetSubprocessContextCancel gets the subprocess context and cancel function for the current
// subprocess context.
func GetSubprocessContextCancel() (context.Context, context.CancelFunc) {
	once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		subprocCtx = &subprocessContext{
			mux:    sync.RWMutex{},
			Ctx:    ctx,
			Cancel: cancel,
		}
	})
	subprocCtx.mux.RLock()
	defer subprocCtx.mux.RUnlock()

	return subprocCtx.Ctx, subprocCtx.Cancel
}

// TerminateRunningProcesses attempts to terminate all tracked running processes gracefully
// context is reset for safety but this function should only be called once, on exit.
func TerminateRunningProcesses() {
	_, cancel := GetSubprocessContextCancel()

	log.Info("Canceling all running python subprocesses")
	cancel()

	// reset context
	subprocCtx.mux.Lock()
	defer subprocCtx.mux.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	subprocCtx.Ctx = ctx
	subprocCtx.Cancel = cancel
}
