// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/checkcontext"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckContext stores the global context required by foreign-runtime submit callbacks.
type CheckContext = checkcontext.CheckContext

// GetCheckContext retrives the current context
func GetCheckContext() (*CheckContext, error) {
	return checkcontext.GetCheckContext()
}

// InitializeCheckContext creates the context that can be later used for storing/retrieving checks context for submit functions
func InitializeCheckContext(senderManager sender.SenderManager, logReceiver option.Option[integrations.Component], tagger tagger.Component, filterStore workloadfilter.Component) {
	checkcontext.InitializeCheckContext(senderManager, logReceiver, tagger, filterStore)
}

// RegisterCheckSenderManager routes rtloader callbacks for id through senderManager
// and returns an idempotent unregister function for the route.
func RegisterCheckSenderManager(id checkid.ID, senderManager sender.SenderManager) (func(), bool) {
	return checkcontext.RegisterCheckSenderManager(id, senderManager)
}
