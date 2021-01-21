// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package tag

import (
	"sync"
	"time"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Provider returns a list of up-to-date tags for a given entity.
type Provider interface {
	GetTags() []string
}

var (
	// NoopProvider does nothing
	NoopProvider Provider = &noopProvider{}
)

// provider provides a list of up-to-date tags for a given entity by calling the tagger.
type provider struct {
	entityID             string
	taggerWarmupDuration time.Duration
	expectedTagsDuration time.Duration
	submitExpectedTags   bool
	sync.Once
	sync.RWMutex
}

// NewProvider returns a new Provider.
func NewProvider(entityID string) Provider {
	p := &provider{
		entityID:             entityID,
		taggerWarmupDuration: config.TaggerWarmupDuration(),
	}

	if config.IsExpectedTagsSet() {
		p.submitExpectedTags = true
		p.expectedTagsDuration = time.Duration(coreConfig.Datadog.GetInt("logs_config.expected_tags_duration")) * time.Minute
	}

	return p
}

// GetTags returns the list of up-to-date tags.
func (p *provider) GetTags() []string {
	p.Do(func() {
		// Warmup duration
		// Make sure the tagger collects all the service tags
		// TODO: remove this once AD and Tagger use the same PodWatcher instance
		<-time.After(p.taggerWarmupDuration)

		// start timer if necessary
		go func() {
			<-time.After(p.expectedTagsDuration)

			p.Lock()
			defer p.Unlock()
			p.submitExpectedTags = false
		}()
	})

	tags, err := tagger.Tag(p.entityID, collectors.HighCardinality)
	if err != nil {
		log.Warnf("Cannot tag container %s: %v", p.entityID, err)
		return []string{}
	}

	p.RLock()
	defer p.RUnlock()
	if p.submitExpectedTags {
		hostTags := host.GetHostTags(true)
		tags = append(tags, hostTags.System...)
	}

	return tags
}
