// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package tests

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Resolver represents a cache resolver
type FakeResolver struct {
	sync.Mutex
	containerIDs []string
	imageIDs     []string
}

// Start the resolver
func (t *FakeResolver) Start(ctx context.Context) error {
	return nil
}

// Stop the resolver
func (t *FakeResolver) Stop() error {
	return nil
}

// Resolve returns the tags for the given id
func (t *FakeResolver) Resolve(containerID string) []string {
	t.Lock()
	defer t.Unlock()
	for index, id := range t.containerIDs {
		if id == containerID {
			return []string{"container_id:" + containerID, fmt.Sprintf("image_name:fake_ubuntu_%d", index+1)}
		}
	}
	t.containerIDs = append(t.containerIDs, containerID)
	return []string{"container_id:" + containerID, fmt.Sprintf("image_name:fake_ubuntu_%d", len(t.containerIDs))}
}

// ResolveWithErr returns the tags for the given id
func (t *FakeResolver) ResolveWithErr(id string) ([]string, error) {
	return t.Resolve(id), nil
}

// GetValue return the tag value for the given id and tag name
func (t *FakeResolver) GetValue(id string, tag string) string {
	return utils.GetTagValue(tag, t.Resolve(id))
}

// ResolveImageMetadata returns the tags for the given image id
func (t *FakeResolver) ResolveImageMetadata(imageID string) []string {
	t.Lock()
	defer t.Unlock()
	for index, id := range t.imageIDs {
		if id == imageID {
			return []string{"container_id:" + imageID, fmt.Sprintf("image_name:fake_ubuntu_%d", index+1)}
		}
	}
	t.imageIDs = append(t.imageIDs, imageID)
	return []string{"container_id:" + imageID, fmt.Sprintf("image_name:fake_ubuntu_%d", len(t.imageIDs))}
}

// GetValueForImage return the tag value for the given id and tag name
func (t *FakeResolver) GetValueForImage(id string, tag string) string {
	return utils.GetTagValue(tag, t.ResolveImageMetadata(id))
}

// Resolove image_id
func (t *FakeResolver) ResolveImageID(containerID string) string {
	imageID := t.GetValue(containerID, "image_id")
	imageName := t.GetValueForImage(imageID, "image_name")
	repoDigests := strings.Split(t.GetValueForImage(imageID, "image_repo_digests"), ",")
	repoTags := strings.Split(t.GetValueForImage(imageID, "image_repo_tags"), ",")

	// If the 'sha256:' prefix is missing, add it
	if !strings.HasPrefix(imageID, "sha256:") {
		imageID = "sha256:" + imageID
	}

	// Build new imageId (repo + @sha256:XXX) or (name + @sha256:XXX) if repo is empty
	// To get repo, check repoDigests first. If empty, check repoTags
	if len(repoDigests) != 0 {
		repo := strings.SplitN(repoDigests[0], "@sha256:", 2)[0]
		if len(repo) != 0 {
			return repo + "@" + imageID
		}
	}
	if len(repoTags) != 0 {
		repo := strings.SplitN(repoDigests[0], ":", 2)[0]
		if len(repo) != 0 {
			return repo + "@" + imageID
		}
	}

	if len(imageName) != 0 {
		return imageName + "@" + imageID
	}
	// If no repo and no image name
	return imageID
}

// NewFakeResolver returns a new tags resolver
func NewFakeResolver() tags.Resolver {
	return &FakeResolver{}
}
