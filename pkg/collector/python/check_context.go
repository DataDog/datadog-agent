// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import (
	"errors"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

var checkCtx *checkContext
var checkContextMutex = sync.Mutex{}

// As it is difficult to pass Go context to Go methods like SubmitMetric,
// checkContext stores the global context required by these functions.
// Doing so allow to have a single global state instead of having one
// per dependency used inside SubmitMetric like methods.
type checkContext struct {
	senderManager sender.SenderManager
}

func getCheckContext() (*checkContext, error) {
	checkContextMutex.Lock()
	defer checkContextMutex.Unlock()

	if checkCtx == nil {
		return nil, errors.New("Python check context was not set")
	}
	return checkCtx, nil
}

func initializeCheckContext(senderManager sender.SenderManager) {
	checkContextMutex.Lock()
	if checkCtx == nil {
		checkCtx = &checkContext{senderManager: senderManager}
	}
	checkContextMutex.Unlock()
}

func releaseCheckContext() {
	checkContextMutex.Lock()
	checkCtx = nil
	checkContextMutex.Unlock()
}
