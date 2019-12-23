// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package tag

import (
	"reflect"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const refreshPeriod = 10 * time.Second

// Provider returns a list of up-to-date tags for a given entity.
type Provider interface {
	GetTags() []string
	Start()
	Stop()
}

// NoopProvider does nothing
var NoopProvider Provider = &noopProvider{}

// provider caches a list of up-to-date tags for a given entity polling periodically the tagger.
type provider struct {
	entityID             string
	tags                 []string
	done                 chan struct{}
	forceRefresh         bool
	forceRefreshDuration time.Duration
	taggerWarmupDuration time.Duration
	mu                   sync.Mutex
	sync.Once
}

// NewProvider returns a new Provider.
func NewProvider(entityID string) Provider {
	p := &provider{
		entityID: entityID,
		tags:     []string{},
		done:     make(chan struct{}),
	}
	config := config.TagProviderConfig()
	p.forceRefresh = config.ForceRefresh
	p.forceRefreshDuration = config.ForceRefreshDuration
	p.taggerWarmupDuration = config.TaggerWarmupDuration
	return p
}

// GetTags returns the list of up-to-date tags.
// if logs_config.k8s_wait_for_tags enabled:
//   - GetTags waits on its first call before updating the local cache for 2 seconds (default)
//   - GetTags forces updating the local cache for 5 seconds (default)
func (p *provider) GetTags() []string {
	p.Do(func() {
		// Warmup duration
		// Make sure the tagger collects all the service tags
		<-time.After(p.taggerWarmupDuration)
	})
	if p.getForceRefresh() {
		p.updateTags()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tags
}

// setForceRefresh udpates the forceRefresh attribute
func (p *provider) setForceRefresh(b bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.forceRefresh = b
}

// getForceRefresh returns the forceRefresh attribute
func (p *provider) getForceRefresh() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.forceRefresh
}

// Start starts the polling of new tags on another go routine.
// Start calls the updateTags method before it returns to make sure
// the local tag cache is populated when GetTags is called
func (p *provider) Start() {
	p.updateTags()
	go func() {
		select {
		case <-p.done:
			return
		case <-time.After(p.forceRefreshDuration):
			// Block updating tags cache
			// during this time GetTags forces the update
			p.setForceRefresh(false)
			break
		}
		ticker := time.NewTicker(refreshPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.updateTags()
			case <-p.done:
				return
			}
		}
	}()
}

// Stop stops the polling of new tags.
func (p *provider) Stop() {
	p.done <- struct{}{}
}

// updateTags updates the list of tags using tagger.
func (p *provider) updateTags() {
	p.mu.Lock()
	defer p.mu.Unlock()
	tags, err := tagger.Tag(p.entityID, collectors.HighCardinality)
	if err != nil {
		log.Debugf("Cannot tag container %s: %v", p.entityID, err)
		return
	}
	if !reflect.DeepEqual(tags, p.tags) {
		p.tags = tags
	}
}
