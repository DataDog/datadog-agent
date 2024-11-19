// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package tests holds tests related files
package tests

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
)

// This fake tagger will give a different image_name for each different container ID

// FakeTagger represents a fake tagger
type FakeTagger struct {
	sync.Mutex
	containerIDs []string
}

// Start the tagger
func (fr *FakeTagger) Start(_ context.Context) error {
	return nil
}

// Stop the tagger
func (fr *FakeTagger) Stop() error {
	return nil
}

// Tag returns the tags for the given id
func (fr *FakeTagger) Tag(entity types.EntityID, _ types.TagCardinality) ([]string, error) {
	containerID := entity.GetID()
	fakeTags := []string{
		"image_tag:latest",
	}
	fr.Lock()
	defer fr.Unlock()
	for index, id := range fr.containerIDs {
		if id == containerID {
			return append(fakeTags, fmt.Sprintf("image_name:fake_ubuntu_%d", index+1)), nil
		}
	}
	fr.containerIDs = append(fr.containerIDs, containerID)
	return append(fakeTags, fmt.Sprintf("image_name:fake_ubuntu_%d", len(fr.containerIDs))), nil
}

// NewFakeTaggerDifferentImageNames returns a new tagger
func NewFakeTaggerDifferentImageNames() tags.Tagger {
	return &FakeTagger{}
}

// This fake tagger will allways give the same image_name, no matter the container ID

// FakeMonoTagger represents a fake mono tagger
type FakeMonoTagger struct{}

// Start the tagger
func (fmr *FakeMonoTagger) Start(_ context.Context) error {
	return nil
}

// Stop the tagger
func (fmr *FakeMonoTagger) Stop() error {
	return nil
}

// Tag returns the tags for the given id
func (fmr *FakeMonoTagger) Tag(entity types.EntityID, _ types.TagCardinality) ([]string, error) {
	return []string{"container_id:" + entity.GetID(), "image_name:fake_ubuntu", "image_tag:latest"}, nil
}

// NewFakeMonoTagger returns a new tags tagger
func NewFakeMonoTagger() tags.Tagger {
	return &FakeMonoTagger{}
}

// This fake tagger will let us specify the next containerID to be resolved

// FakeManualTagger represents a fake manual tagger
type FakeManualTagger struct {
	sync.Mutex
	containerToSelector map[string]*cgroupModel.WorkloadSelector
	cpt                 int
	nextSelectors       []*cgroupModel.WorkloadSelector
}

// Start the tagger
func (fmr *FakeManualTagger) Start(_ context.Context) error {
	return nil
}

// Stop the tagger
func (fmr *FakeManualTagger) Stop() error {
	return nil
}

// SpecifyNextSelector specifies the next image name and tag to be resolved
func (fmr *FakeManualTagger) SpecifyNextSelector(selector *cgroupModel.WorkloadSelector) {
	fmr.Lock()
	defer fmr.Unlock()
	fmr.nextSelectors = append(fmr.nextSelectors, selector)
}

// GetContainerSelector returns the container selector
func (fmr *FakeManualTagger) GetContainerSelector(containerID string) *cgroupModel.WorkloadSelector {
	fmr.Lock()
	defer fmr.Unlock()
	if selector, found := fmr.containerToSelector[containerID]; found {
		return selector
	}
	return nil
}

// Tag returns the tags for the given id
func (fmr *FakeManualTagger) Tag(entity types.EntityID, _ types.TagCardinality) ([]string, error) {
	fmr.Lock()
	defer fmr.Unlock()

	containerID := entity.GetID()
	// first, use cache if any
	selector, alreadyResolved := fmr.containerToSelector[containerID]
	if alreadyResolved {
		return []string{"container_id:" + containerID, "image_name:" + selector.Image, "image_tag:" + selector.Tag}, nil
	}

	// if no cache and there is a pending list, use it
	if len(fmr.nextSelectors) > 0 {
		selector = fmr.nextSelectors[0]
		fmr.nextSelectors = fmr.nextSelectors[1:]
		fmr.containerToSelector[containerID] = selector
		return []string{"container_id:" + containerID, "image_name:" + selector.Image, "image_tag:" + selector.Tag}, nil
	}

	// otherwise generate a new selector
	fmr.cpt++
	selector = &cgroupModel.WorkloadSelector{
		Image: fmt.Sprintf("fake_name_%d", fmr.cpt),
		Tag:   "fake_tag",
	}
	fmr.containerToSelector[containerID] = selector
	return []string{"container_id:" + containerID, "image_name:" + selector.Image, "image_tag:" + selector.Tag}, nil
}

// NewFakeManualTagger returns a new tagger
func NewFakeManualTagger() *FakeManualTagger {
	return &FakeManualTagger{
		containerToSelector: make(map[string]*cgroupModel.WorkloadSelector),
	}
}
