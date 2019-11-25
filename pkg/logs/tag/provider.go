// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package tag

import (
	"reflect"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
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
	entityID string
	tags     []string
	done     chan struct{}
	mu       sync.Mutex
}

// NewProvider returns a new Provider.
func NewProvider(entityID string) Provider {
	return &provider{
		entityID: entityID,
		tags:     []string{},
		done:     make(chan struct{}),
	}
}

// GetTags returns the list of up-to-date tags.
func (p *provider) GetTags() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tags
}

// Start starts the polling of new tags on another go routine.
func (p *provider) Start() {
	go func() {
		p.updateTags()
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
	if err == nil && !reflect.DeepEqual(tags, p.tags) {
		p.tags = tags
	}
}
