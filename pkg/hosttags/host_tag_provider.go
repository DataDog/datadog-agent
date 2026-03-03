// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hosttags provides a mechanism to fetch host tags for metrics in the Datadog Agent.
package hosttags

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/hosttags"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/benbjohnson/clock"
)

// HostTagProvider is a struct that provides host tags for metrics.
type HostTagProvider struct {
	hostTags atomic.Pointer[[]string]
}

// NewHostTagProvider creates a new HostTagProvider with the default expected tags duration from the configuration.
func NewHostTagProvider() *HostTagProvider {
	return NewHostTagProviderWithDuration(pkgconfigsetup.Datadog().GetDuration("expected_tags_duration"))
}

// NewHostTagProviderWithDuration creates a new HostTagProvider with a specified duration for host tags expiration.
func NewHostTagProviderWithDuration(duration time.Duration) *HostTagProvider {
	return newHostTagProviderWithClock(clock.New(), duration)
}

func newHostTagProviderWithClock(clock clock.Clock, duration time.Duration) *HostTagProvider {
	p := &HostTagProvider{}

	log.Debugf("Adding host tags to metrics for %v", duration)
	if duration > 0 {
		tags := slices.Clone(hostMetadataUtils.Get(context.TODO(), false, pkgconfigsetup.Datadog()).System)
		p.hostTags.Store(&tags)
		expectedTagsDeadline := pkgconfigsetup.StartTime.Add(duration)
		clock.AfterFunc(expectedTagsDeadline.Sub(clock.Now()), func() {
			p.hostTags.Store(nil)
			log.Debugf("host tags for metrics have expired")
		})
	}

	return p
}

// GetHostTags returns the current host tags. If the tags have expired, it returns nil.
func (p *HostTagProvider) GetHostTags() []string {
	if ptr := p.hostTags.Load(); ptr != nil {
		return *ptr
	}
	return nil
}
