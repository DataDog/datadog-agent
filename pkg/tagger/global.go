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

// checksFullCardinality holds the config that says whether we should send high cardinality tags for checks metrics
// this can still be overriden when calling get_tags in python checks.
var checksFullCardinality bool

// dogstatsdFullCardinality holds the config that says whether we should send high cardinality tags for metrics from
// dogstatsd.
var dogstatsdFullCardinality bool

// Init must be called once config is available, call it in your cmd
// defaultTagger.Init cannot fail for now, keeping the `error` for API stability
func Init() error {
	initOnce.Do(func() {
		checksFullCardinality = config.Datadog.GetBool("send_container_tags_for_checks")
		dogstatsdFullCardinality = config.Datadog.GetBool("send_container_tags_for_dogstatsd")
		defaultTagger.Init(collectors.DefaultCatalog)
	})
	return nil
}

// Tag queries the defaultTagger to get entity tags from cache or sources.
// It can return tags at high cardinality (with tags about individual containers),
// or at orchestrator cardinality (pod/task level)
func Tag(entity string, highCard bool) ([]string, error) {
	if highCard == true {
		return defaultTagger.Tag(entity, collectors.HighCardinality)
	} else {
		return defaultTagger.Tag(entity, collectors.OrchestratorCardinality)
	}
}

// Stop queues a stop signal to the defaultTagger
func Stop() error {
	return defaultTagger.Stop()
}

// IsChecksFullCardinality returns the full_cardinality_tagging option
// this caches the call to viper, that would lookup and parse envvars
func IsChecksFullCardinality() bool {
	return checksFullCardinality
}

// IsDogstatsdFullCardinality returns the full_cardinality_tagging option
// this caches the call to viper, that would lookup and parse envvars
func IsDogstatsdFullCardinality() bool {
	return dogstatsdFullCardinality
}

// List the content of the defaulTagger
func List(highCard bool) response.TaggerListResponse {
	if highCard == true {
		return defaultTagger.List(collectors.HighCardinality)
	} else {
		return defaultTagger.List(collectors.OrchestratorCardinality)
	}
}

// GetEntityHash returns the hash for the tags associated with the given entity
func GetEntityHash(entity string) string {
	return defaultTagger.GetEntityHash(entity)
}

func init() {
	defaultTagger = newTagger()
}
