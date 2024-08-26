// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Provider returns a list of up-to-date tags for a given entity.
type Provider interface {
	GetTags() []string
}

// provider provides a list of up-to-date tags for a given entity by calling the tagger.
type provider struct {
	entityID             string
	taggerWarmupDuration time.Duration
	localTagProvider     Provider
	clock                clock.Clock
	sync.Once
}

// NewProvider returns a new Provider.
func NewProvider(entityID string) Provider {
	return newProviderWithClock(entityID, clock.New())
}

// newProviderWithClock returns a new provider using the given clock.
func newProviderWithClock(entityID string, clock clock.Clock) Provider {
	p := &provider{
		entityID:             entityID,
		taggerWarmupDuration: config.TaggerWarmupDuration(pkgConfig.Datadog()),
		localTagProvider:     newLocalProviderWithClock([]string{}, clock),
		clock:                clock,
	}

	return p
}

// GetTags returns the list of up-to-date tags.
func (p *provider) GetTags() []string {
	p.Do(func() {
		// Warmup duration
		// Make sure the tagger collects all the service tags
		// TODO: remove this once AD and Tagger use the same PodWatcher instance
		p.clock.Sleep(p.taggerWarmupDuration)
	})

	tags, err := tagger.Tag(p.entityID, types.HighCardinality)
	if err != nil {
		log.Warnf("Cannot tag container %s: %v", p.entityID, err)
		return []string{}
	}

	localTags := p.localTagProvider.GetTags()
	if localTags != nil {
		tags = append(tags, localTags...)
	}

	return tags
}
