// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package tagger

import (
	"sync"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

// defaultTagger is the shared tagger instance backing the global Tag and Init functions
var defaultTagger *Tagger
var initOnce sync.Once

// fullCardinality caches the value of the full_cardinality_taggin option
var fullCardinality bool

// Init must be called once config is available, call it in your cmd
// defaultTagger.Init cannot fail for now, keeping the `error` for API stability
func Init() error {
	initOnce.Do(func() {
		fullCardinality = config.Datadog.GetBool("full_cardinality_tagging")
		defaultTagger.Init(collectors.DefaultCatalog)
	})
	return nil
}

// Tag queries the defaultTagger to get entity tags from cache or sources
func Tag(entity string, highCard bool) ([]string, error) {
	return defaultTagger.Tag(entity, highCard)
}

// Stop queues a stop signal to the defaultTagger
func Stop() error {
	return defaultTagger.Stop()
}

// IsFullCardinality returns the full_cardinality_tagging option
// this caches the call to viper, that would lookup and parse envvars
func IsFullCardinality() bool {
	return fullCardinality
}

// List the content of the defaulTagger
func List(highCard bool) response.TaggerListResponse {
	return defaultTagger.List(highCard)
}

// OutdatedTags returns a boolean based on high cards tags.
func OutdatedTags(ADIdentifiers []string) bool {
	return defaultTagger.OutdatedTags(ADIdentifiers)
}

func init() {
	defaultTagger = newTagger()
}
