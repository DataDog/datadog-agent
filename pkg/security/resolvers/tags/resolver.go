// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"context"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// Tagger defines a Tagger for the Tags Resolver
type Tagger interface {
	Init(context.Context) error
	Stop() error
	Tag(entity string, cardinality collectors.TagCardinality) ([]string, error)
}

type nullTagger struct{}

func (n *nullTagger) Init(context.Context) error {
	return nil
}

func (n *nullTagger) Stop() error {
	return nil
}

func (n *nullTagger) Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	return nil, nil
}

// Resolver represents a cache resolver
type Resolver interface {
	Start(ctx context.Context) error
	Stop() error
	Resolve(id string) []string
	ResolveWithErr(id string) ([]string, error)
	GetValue(id string, tag string) string
}

type DefaultResolver struct {
	tagger Tagger
}

// Start the resolver
func (t *DefaultResolver) Start(ctx context.Context) error {
	go func() {
		if err := t.tagger.Init(ctx); err != nil {
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
func (t *DefaultResolver) Resolve(id string) []string {
	tags, _ := t.tagger.Tag("container_id://"+id, collectors.OrchestratorCardinality)
	return tags
}

// ResolveWithErr returns the tags for the given id
func (t *DefaultResolver) ResolveWithErr(id string) ([]string, error) {
	return t.tagger.Tag("container_id://"+id, collectors.OrchestratorCardinality)
}

// GetValue return the tag value for the given id and tag name
func (t *DefaultResolver) GetValue(id string, tag string) string {
	return utils.GetTagValue(tag, t.Resolve(id))
}

// Stop the resolver
func (t *DefaultResolver) Stop() error {
	return t.tagger.Stop()
}

// NewResolver returns a new tags resolver
func NewResolver(config *config.Config) Resolver {
	if config.RemoteTaggerEnabled {
		options, err := remote.NodeAgentOptions()
		if err != nil {
			log.Errorf("unable to configure the remote tagger: %s", err)
		} else {
			return &DefaultResolver{
				tagger: remote.NewTagger(options),
			}
		}
	}
	return &DefaultResolver{
		tagger: &nullTagger{},
	}
}

// Resolove image_id
func (t *Resolver) ResolveImageID(containerID string) (string, error) {
	m := workloadmeta.GetGlobalStore()

	// Get imageID from container so that we can get the image Metadata
	container, err := m.GetContainer(containerID)
	if err != nil {
		log.Errorf("unable to get container %s: %s", containerID, err)
		return "", err
	}

	imageID := container.Image.ID
	imageName := container.Image.Name

	// Build new imageId (repo + @sha256:XXX) or (name + @sha256:XXX) if repo is empty
	// To get repo, check repoDigests first. If empty, check repoTags
	imageMetadata, err := m.GetImage(imageID)
	if err != nil {
		log.Errorf("unable get image metadata for %s: %s", imageID, err)
	} else {
		// If the 'sha256:' prefix is missing, add it
		if !strings.HasPrefix(imageID, "sha256:") {
			imageID = "sha256:" + imageID
		}
		repoDigests := imageMetadata.RepoDigests
		repoTags := imageMetadata.RepoTags
		if len(repoDigests) != 0 {
			repo := strings.SplitN(repoDigests[0], "@sha256:", 2)[0]
			return repo + "@" + imageID, nil
		}
		if len(repoTags) != 0 {
			repo := strings.SplitN(repoDigests[0], ":", 2)[0]
			return repo + "@" + imageID, nil
		}
	}
	// If repo is empty or if could not find it, return imageName+@+imageID
	return imageName + "@" + imageID, nil
}
