// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"errors"
	"sync"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
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
	logReceiver   option.Option[integrations.Component]
	tagger        tagger.Component
	filter        workloadfilter.FilterBundle
}

func (cc *CheckContext) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	return cc.tagger.Tag(entityID, cardinality)
}

func (cc *CheckContext) GetLogReceiver() (integrations.Component, bool) {
	return cc.logReceiver.Get()
}

func (cc *CheckContext) IsExcluded(container *workloadfilter.Container) bool {
	return cc.filter.IsExcluded(container)
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
			logReceiver:   logReceiver,
			tagger:        tagger,
			filter:        filterStore.GetContainerSharedMetricFilters(),
		}

		if _, ok := logReceiver.Get(); !ok {
			log.Warn("Log receiver not provided. Logs from integrations will not be collected.")
		}
	}

	checkContextMutex.Unlock()
}
