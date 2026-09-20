// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hosttags provides a mechanism to fetch host tags for metrics in the Datadog Agent.
package hosttags

import (
	"context"
	"sync"
	"time"

	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/impl/hosttags"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	k8shostinfo "github.com/DataDog/datadog-agent/pkg/util/kubernetes/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/benbjohnson/clock"
)

// reservedTagsInterval is the refresh cadence of the tag-rule tags: the
// cluster-agent tag-rule controller can flip their values at a 15s debounce,
// so a matching refresh keeps the metrics view live without multiplying
// apiserver load (one single-object GET per agent per interval).
const reservedTagsInterval = 15 * time.Second

// HostTagProvider is a struct that provides host tags for metrics.
type HostTagProvider struct {
	hostTags []string
	// reservedTags are the tag-rule tags: rule-owned, refreshed every
	// reservedTagsInterval, and attached to metrics for the process lifetime
	// regardless of the expected_tags_duration window.
	reservedTags []string
	sync.RWMutex
}

// NewHostTagProvider creates a new HostTagProvider with the default expected tags duration from the configuration.
func NewHostTagProvider() *HostTagProvider {
	return NewHostTagProviderWithDuration(setup.Datadog().GetDuration("expected_tags_duration"))
}

// NewHostTagProviderWithDuration creates a new HostTagProvider with a specified duration for host tags expiration.
func NewHostTagProviderWithDuration(duration time.Duration) *HostTagProvider {
	return newHostTagProviderWithClock(clock.New(), duration)
}

func newHostTagProviderWithClock(clock clock.Clock, duration time.Duration) *HostTagProvider {
	return newHostTagProviderWithClockAndFetch(clock, duration, fetchTagRuleTags)
}

// fetchTagRuleTags is the default source of the tag-rule tags.
func fetchTagRuleTags() ([]string, error) {
	return k8shostinfo.GetTagRuleNodeTags(context.Background())
}

func newHostTagProviderWithClockAndFetch(clock clock.Clock, duration time.Duration, fetch func() ([]string, error)) *HostTagProvider {
	p := &HostTagProvider{
		hostTags: nil,
	}

	log.Debugf("Adding host tags to metrics for %v", duration)
	if duration > 0 {
		p.hostTags = cloneTags(hostMetadataUtils.Get(context.TODO(), false, setup.Datadog()).System)
		expectedTagsDeadline := setup.StartTime.Add(duration)
		clock.AfterFunc(expectedTagsDeadline.Sub(clock.Now()), func() {
			p.Lock()
			defer p.Unlock()
			p.hostTags = nil
			log.Debugf("host tags for metrics have expired")
		})
	}

	// Tag-rule tags are refreshed for the provider's lifetime: the fetch runs
	// asynchronously so a slow first fetch never delays agent startup, and a
	// failed refresh keeps the last known tags (a fetch error must not drop
	// tag-rule tags from metrics).
	go func() {
		if tags, err := fetch(); err == nil {
			p.setReservedTags(tags)
		} else {
			log.Debugf("Unable to collect tag-rule node tags: %s", err)
		}
		ticker := clock.Ticker(reservedTagsInterval)
		for range ticker.C {
			if tags, err := fetch(); err == nil {
				p.setReservedTags(tags)
			} else {
				log.Debugf("Unable to refresh tag-rule node tags, keeping last known: %s", err)
			}
		}
	}()

	return p
}

func (p *HostTagProvider) setReservedTags(tags []string) {
	p.Lock()
	defer p.Unlock()
	p.reservedTags = cloneTags(tags)
}

// GetHostTags returns the current host tags: the windowed snapshot plus the
// tag-rule tags, which never expire.
func (p *HostTagProvider) GetHostTags() []string {
	p.RLock()
	defer p.RUnlock()

	if len(p.reservedTags) == 0 {
		return p.hostTags
	}
	if len(p.hostTags) == 0 {
		return p.reservedTags
	}
	tags := make([]string, 0, len(p.hostTags)+len(p.reservedTags))
	tags = append(tags, p.hostTags...)
	tags = append(tags, p.reservedTags...)
	return tags
}

func cloneTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	clone := make([]string, len(tags))
	copy(clone, tags)
	return clone
}
