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

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Resolver represents a cache resolver
type FakeResolver struct {
	containerIDs []string
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
	for index, id := range t.containerIDs {
		if id == containerID {
			return []string{"container_id:" + containerID, "image_name:fake_ubuntu_" + fmt.Sprint(index+1)}
		}
	}
	t.containerIDs = append(t.containerIDs, containerID)
	return []string{"container_id:" + containerID, "image_name:fake_ubuntu_" + fmt.Sprint(len(t.containerIDs))}
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
