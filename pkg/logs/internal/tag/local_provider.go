// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package tag

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	coreConfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/benbjohnson/clock"
)

// NOTE: to avoid races, do not modify the contents of the `expectedTags`
// slice, as those contents are referenced without holding the lock.

type localProvider struct {
	tags         []string
	expectedTags []string
	sync.RWMutex
}

// NewLocalProvider returns a new local Provider.
func NewLocalProvider(t []string) Provider {
	return newLocalProviderWithClock(t, clock.New())
}

// newLocalProviderWithClock returns a provider using the given clock.
func newLocalProviderWithClock(t []string, clock clock.Clock) Provider {
	p := &localProvider{
		tags:         t,
		expectedTags: t,
	}

	if config.IsExpectedTagsSet(coreConfig.Datadog) {
		p.expectedTags = append(p.tags, hostMetadataUtils.Get(context.TODO(), false, coreConfig.Datadog).System...)

		// expected tags deadline is based on the agent start time, which may have been earlier
		// than the current time.
		expectedTagsDeadline := coreConfig.StartTime.Add(coreConfig.Datadog.GetDuration("logs_config.expected_tags_duration"))

		// reset submitExpectedTags after deadline elapsed
		clock.AfterFunc(expectedTagsDeadline.Sub(clock.Now()), func() {
			p.Lock()
			defer p.Unlock()
			p.expectedTags = nil
		})
	}

	return p
}

// GetTags returns the list of locally-configured tags.  This will include the
// expected tags until the expected-tags deadline, if those are configured.  The
// returned slice is shared and must not be mutated.
func (p *localProvider) GetTags() []string {
	p.RLock()
	defer p.RUnlock()

	if p.expectedTags != nil {
		return p.expectedTags
	}
	return p.tags
}
