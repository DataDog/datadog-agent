// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Tagger defines a Tagger for the Tags Resolver
type Tagger interface {
	Init() error
	Stop() error
	Tag(entity string, cardinality collectors.TagCardinality) ([]string, error)
}

type nullTagger struct{}

func (n *nullTagger) Init() error {
	return nil
}

func (n *nullTagger) Stop() error {
	return nil
}

func (n *nullTagger) Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	return nil, nil
}

// TagsResolver represents a cache resolver
type TagsResolver struct {
	tagger Tagger
}

// Start the resolver
func (t *TagsResolver) Start(ctx context.Context) error {
	go func() {
		if err := t.tagger.Init(); err != nil {
			log.Errorf("failed to init tagger: %s", err)
		}
	}()

	go func() {
		<-ctx.Done()
		_ = t.tagger.Stop()
	}()

	return nil
}

// Resolve returns the tags for the given id
func (t *TagsResolver) Resolve(id string) []string {
	tags, _ := t.tagger.Tag("container_id://"+id, collectors.OrchestratorCardinality)
	return tags
}

// GetValue return the tag value for the given id and tag name
func (t *TagsResolver) GetValue(id string, tag string) string {
	return utils.GetTagValue(tag, t.Resolve(id))
}

// Stop the resolver
func (t *TagsResolver) Stop() error {
	return t.tagger.Stop()
}

// NewTagsResolver returns a new tags resolver
func NewTagsResolver(config *config.Config) *TagsResolver {
	var tagger Tagger
	if config.RemoteTaggerEnabled {
		tagger = remote.NewTagger()
	} else {
		tagger = &nullTagger{}
	}

	return &TagsResolver{
		tagger: tagger,
	}
}
