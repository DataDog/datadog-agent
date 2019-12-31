// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package tag

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Provider returns a list of up-to-date tags for a given entity.
type Provider interface {
	GetTags() []string
}

// NoopProvider does nothing
var NoopProvider Provider = &noopProvider{}

// provider provides a list of up-to-date tags for a given entity by calling the tagger.
type provider struct {
	entityID             string
	taggerWarmupDuration time.Duration
	sync.Once
}

// NewProvider returns a new Provider.
func NewProvider(entityID string) Provider {
	return &provider{
		entityID:             entityID,
		taggerWarmupDuration: config.TaggerWarmupDuration(),
	}
}

// GetTags returns the list of up-to-date tags.
func (p *provider) GetTags() []string {
	p.Do(func() {
		// Warmup duration
		// Make sure the tagger collects all the service tags
		// TODO: remove this once AD and Tagger use the same PodWatcher instance
		<-time.After(p.taggerWarmupDuration)
	})
	tags, err := tagger.Tag(p.entityID, collectors.HighCardinality)
	if err != nil {
		log.Warnf("Cannot tag container %s: %v", p.entityID, err)
		return []string{}
	}
	return tags
}
