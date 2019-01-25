// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package tagger

import (
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultTagger is the shared tagger instance backing the global Tag and Init functions
var defaultTagger *Tagger
var initOnce sync.Once

// ChecksCardinality defines the cardinality of tags we should send for check metrics
// this can still be overridden when calling get_tags in python checks.
var ChecksCardinality collectors.TagCardinality

// DogstatsdCardinality defines the cardinality of tags we should send for metrics from
// dogstatsd.
var DogstatsdCardinality collectors.TagCardinality

// Init must be called once config is available, call it in your cmd
func Init() {
	initOnce.Do(func() {
		var err error
		checkCard := config.Datadog.GetString("checks_tag_cardinality")
		dsdCard := config.Datadog.GetString("dogstatsd_tag_cardinality")

		ChecksCardinality, err = stringToTagCardinality(checkCard)
		if err != nil {
			log.Warnf("failed to parse check tag cardinality, defaulting to low. Error: %s", err)
			ChecksCardinality = collectors.LowCardinality
		}
		DogstatsdCardinality, err = stringToTagCardinality(dsdCard)
		if err != nil {
			log.Warnf("failed to parse dogstatsd tag cardinality, defaulting to low. Error: %s", err)
			DogstatsdCardinality = collectors.LowCardinality
		}

		defaultTagger.Init(collectors.DefaultCatalog)
	})
}

// Tag queries the defaultTagger to get entity tags from cache or sources.
// It can return tags at high cardinality (with tags about individual containers),
// or at orchestrator cardinality (pod/task level)
func Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	return defaultTagger.Tag(entity, cardinality)
}

// Stop queues a stop signal to the defaultTagger
func Stop() error {
	return defaultTagger.Stop()
}

// List the content of the defaulTagger
func List(cardinality collectors.TagCardinality) response.TaggerListResponse {
	return defaultTagger.List(cardinality)
}

// GetEntityHash returns the hash for the tags associated with the given entity
func GetEntityHash(entity string) string {
	return defaultTagger.GetEntityHash(entity)
}

// stringToTagCardinality extracts a TagCardinality from a string.
// In case of failure to parse, returns an error and defaults to Low.
func stringToTagCardinality(c string) (collectors.TagCardinality, error) {
	switch strings.ToLower(c) {
	case "high":
		return collectors.HighCardinality, nil
	case "orchestrator":
		return collectors.OrchestratorCardinality, nil
	case "low":
		return collectors.LowCardinality, nil
	default:
		return collectors.LowCardinality, fmt.Errorf("unsupported value %s received for tag cardinality", c)
	}
}

func init() {
	defaultTagger = newTagger()
}
