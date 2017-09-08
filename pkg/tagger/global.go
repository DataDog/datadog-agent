// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package tagger

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

// DefaultTagger is the common tagger to be used by all users
var DefaultTagger *Tagger

// Init must be called once config is available, call it in your cmd
func Init() error {
	return DefaultTagger.Init(collectors.DefaultCatalog)
}

// Tag queries the defaulttagger to get entity tags from cache or sources
func Tag(entity string, highCard bool) ([]string, error) {
	return DefaultTagger.Tag(entity, highCard)
}

// Stop queues a stop signal to the defaulttagger
func Stop() error {
	return DefaultTagger.Stop()
}

func init() {
	tagger, err := NewTagger()
	if err != nil {
		log.Errorf("tagger initialisation failed: %s", err)
	}
	DefaultTagger = tagger
}
