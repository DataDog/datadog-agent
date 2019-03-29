// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package tag

// Provider keeps a list of up-to-date tags for a given entity polling periodically the tagger.
type Provider struct {
	entityName string
	tags       []string
	done       chan struct{}
	mu         sync.Mutex
}

// NewProvider returns a new Provider.
func NewProvider(entityName string, refreshPeriod time.Duration) *Provider {
	return &Provider{
		entityName: entityName,
		tags:       []string{},
		done:       make(chan struct{}),
	}
}

// GetTags returns the list of up-to-date tags.
func (p *Provider) GetTags() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tags
}

// Start starts the polling of new tags on another go routine.
func (p *Provider) Start() {
	go func() {
		ticker := NewTicker(refreshPeriod)
		for {
			select {
			case <-ticker.C:
				p.updateTags()
			case <-p.done:
				return
			}
		}
		ticker.Stop()
	}()
}

// Stop stops the polling of new tags.
func (p *Provider) Stop() {
	p.done <- struct{}{}
}

// updateTags updates the list of tags using tagger.
func (p *Provider) updateTags() {
	p.mu.Lock()
	defer p.mu.Unlock()
	tags, err := tagger.Tag(p.entityName, collectors.HighCardinality)
	if err == nil && !reflect.DeepEqual(tags, p.tags) {
		p.tags = tags
	}
}
