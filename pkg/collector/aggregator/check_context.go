// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"errors"
	"sync"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var checkCtx *CheckContext
var checkContextMutex = sync.Mutex{}

// CheckContext stores the global context required by Go methods like SubmitMetric.
// Doing so allow to have a single global state instead of having one
// per dependency used inside SubmitMetric like methods.
type CheckContext struct {
	senderManager sender.SenderManager
	LogReceiver   option.Option[integrations.Component]
	Tagger        tagger.Component
	Filter        workloadfilter.FilterBundle
}

// GetCheckContext retrives the current context
func GetCheckContext() (*CheckContext, error) {
	checkContextMutex.Lock()
	defer checkContextMutex.Unlock()

	if checkCtx == nil {
		return nil, errors.New("Python check context was not set")
	}
	return checkCtx, nil
}

// InitializeCheckContext creates the context that can be later used for storing/retrieving checks context for submit functions
func InitializeCheckContext(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component, filterStore workloadfilter.Component) {
	checkContextMutex.Lock()
	if checkCtx == nil {
		checkCtx = &CheckContext{
			senderManager: senderManager,
			LogReceiver:   logReceiver,
			Tagger:        tagger,
			Filter:        filterStore.GetContainerSharedMetricFilters(),
		}

		if _, ok := logReceiver.Get(); !ok {
			log.Warn("Log receiver not provided. Logs from integrations will not be collected.")
		}
	}

	checkContextMutex.Unlock()
}

// releaseCheckContext is only used in test files
//
//nolint:unused
func releaseCheckContext() {
	checkContextMutex.Lock()
	checkCtx = nil
	checkContextMutex.Unlock()
}
