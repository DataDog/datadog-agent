// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tests holds tests related files
package tests

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// This fake resolver will give a different image_name for each different container ID

// FakeResolver represents a fake cache resolver
type FakeResolver struct {
	sync.Mutex
	containerIDs []string
}

// Start the resolver
func (t *FakeResolver) Start(_ context.Context) error {
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

// NewFakeResolver returns a new tags resolver
func NewFakeResolver() tags.Resolver {
	return &FakeResolver{}
}

// This fake resolver will allways give the same image_name, no matter the container ID

// FakeMonoResolver represents a fake mono resolver
type FakeMonoResolver struct {
}

// Start the resolver
func (t *FakeMonoResolver) Start(_ context.Context) error {
	return nil
}

// Stop the resolver
func (t *FakeMonoResolver) Stop() error {
	return nil
}

// Resolve returns the tags for the given id
func (t *FakeMonoResolver) Resolve(containerID string) []string {
	return []string{"container_id:" + containerID, "image_name:fake_ubuntu"}
}

// ResolveWithErr returns the tags for the given id
func (t *FakeMonoResolver) ResolveWithErr(id string) ([]string, error) {
	return t.Resolve(id), nil
}

// GetValue return the tag value for the given id and tag name
func (t *FakeMonoResolver) GetValue(id string, tag string) string {
	return utils.GetTagValue(tag, t.Resolve(id))
}

// NewFakeMonoResolver returns a new tags resolver
func NewFakeMonoResolver() tags.Resolver {
	return &FakeMonoResolver{}
}
