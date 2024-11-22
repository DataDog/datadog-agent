// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tag

import (
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Provider returns a list of up-to-date tags for a given entity.
type Provider interface {
	GetTags() []string
}

// EntityTagAdder returns the associated tag for an entity and their cardinality
type EntityTagAdder interface {
	Tag(entity types.EntityID, cardinality types.TagCardinality) ([]string, error)
}

// provider provides a list of up-to-date tags for a given entity by calling the tagger.
type provider struct {
	entityID             types.EntityID
	taggerWarmupDuration time.Duration
	localTagProvider     Provider
	clock                clock.Clock
	tagAdder             EntityTagAdder
	sync.Once
}

// NewProvider returns a new Provider.
func NewProvider(entityID types.EntityID, tagAdder EntityTagAdder) Provider {
	return newProviderWithClock(entityID, clock.New(), tagAdder)
}

// newProviderWithClock returns a new provider using the given clock.
func newProviderWithClock(entityID types.EntityID, clock clock.Clock, tagAdder EntityTagAdder) Provider {
	p := &provider{
		entityID:             entityID,
		taggerWarmupDuration: config.TaggerWarmupDuration(pkgconfigsetup.Datadog()),
		localTagProvider:     newLocalProviderWithClock([]string{}, clock),
		clock:                clock,
		tagAdder:             tagAdder,
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

	tags, err := p.tagAdder.Tag(p.entityID, types.HighCardinality)
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
