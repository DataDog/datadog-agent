// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"github.com/DataDog/datadog-agent/comp/logs-library/processor"
	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// startTagFilterSubscriber resolves each source's tag filter as soon as the logs
// agent learns of it, instead of waiting for its first message to reach a
// processor. It runs until done is closed.
func startTagFilterSubscriber(logSources *sources.LogSources, tagFilters *tagfilter.Filters, done chan struct{}) {
	added, removed := logSources.SubscribeAll(done, done)

	go func() {
		for {
			select {
			case src := <-added:
				processor.ResolveSourceTagFilter(tagFilters, src)
			case <-removed:
			case <-done:
				return
			}
		}
	}()
}
