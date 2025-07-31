// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"errors"
	"sync"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var checkCtx *checkContext
var checkContextMutex = sync.Mutex{}

// As it is difficult to pass Go context to Go methods like SubmitMetric,
// checkContext stores the global context required by these functions.
// Doing so allow to have a single global state instead of having one
// per dependency used inside SubmitMetric like methods.
// With the addition of shared library checks, we need to distinguish these types of checks.
type checkContext struct {
	senderManager sender.SenderManager
	logReceiver   option.Option[integrations.Component]
	tagger        tagger.Component
}

func getCheckContext() (*checkContext, error) {
	checkContextMutex.Lock()
	defer checkContextMutex.Unlock()

	if checkCtx == nil {
		return nil, errors.New("Check context was not set")
	}
	return checkCtx, nil
}

// InitializeCheckContext creates a check context when creating the loader to later provide info to checks
func InitializeCheckContext(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component) {
	checkContextMutex.Lock()
	if checkCtx == nil {
		checkCtx = &checkContext{
			senderManager: senderManager,
			logReceiver:   logReceiver,
			tagger:        tagger,
		}

		if _, ok := logReceiver.Get(); !ok {
			log.Warn("Log receiver not provided. Logs from integrations will not be collected.")
		}
	}

	checkContextMutex.Unlock()
}

// ReleaseCheckContext erases the check context
func ReleaseCheckContext() {
	checkContextMutex.Lock()
	checkCtx = nil
	checkContextMutex.Unlock()
}
