// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package tagger

import (
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

// defaultTagger is the shared tagger instance backing the global Tag and Init functions
var defaultTagger *Tagger

// Init must be called once config is available, call it in your cmd
func Init() error {
	return defaultTagger.Init(collectors.DefaultCatalog)
}

// Tag queries the defaultTagger to get entity tags from cache or sources
func Tag(entity string, highCard bool) ([]string, error) {
	return defaultTagger.Tag(entity, highCard)
}

// Stop queues a stop signal to the defaultTagger
func Stop() error {
	return defaultTagger.Stop()
}

func init() {
	defaultTagger = newTagger()
}
