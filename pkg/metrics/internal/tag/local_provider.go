// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive
package tag

import (
	"context"
	"fmt"
	"sync"

	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

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
func NewLocalProvider(t []string) *localProvider {
	return newLocalProviderWithClock(t, clock.New())
}

// newLocalProviderWithClock returns a provider using the given clock.
func newLocalProviderWithClock(t []string, clock clock.Clock) *localProvider {
	p := &localProvider{
		tags:         t,
		expectedTags: t,
	}
	duration := pkgconfigsetup.Datadog().GetDuration("expected_tags_duration")
	fmt.Println("aids", duration, duration > 0)
	if pkgconfigsetup.Datadog().GetDuration("expected_tags_duration") > 0 {
		p.expectedTags = append(p.tags, hostMetadataUtils.Get(context.TODO(), false, pkgconfigsetup.Datadog()).System...)
		fmt.Println("WACK7 expected tags are:", p.expectedTags)
		// expected tags deadline is based on the agent start time, which may have been earlier
		// than the current time.
		expectedTagsDeadline := pkgconfigsetup.StartTime.Add(duration)

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
