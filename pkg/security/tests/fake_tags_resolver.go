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

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
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
func (fr *FakeResolver) Start(_ context.Context) error {
	return nil
}

// Stop the resolver
func (fr *FakeResolver) Stop() error {
	return nil
}

// Resolve returns the tags for the given id
func (fr *FakeResolver) Resolve(containerID string) []string {
	fakeTags := []string{
		"image_tag:latest",
	}
	fr.Lock()
	defer fr.Unlock()
	for index, id := range fr.containerIDs {
		if id == containerID {
			return append(fakeTags, fmt.Sprintf("image_name:fake_ubuntu_%d", index+1))
		}
	}
	fr.containerIDs = append(fr.containerIDs, containerID)
	return append(fakeTags, fmt.Sprintf("image_name:fake_ubuntu_%d", len(fr.containerIDs)))
}

// ResolveWithErr returns the tags for the given id
func (fr *FakeResolver) ResolveWithErr(id string) ([]string, error) {
	return fr.Resolve(id), nil
}

// GetValue return the tag value for the given id and tag name
func (fr *FakeResolver) GetValue(id string, tag string) string {
	return utils.GetTagValue(tag, fr.Resolve(id))
}

// NewFakeResolverDifferentImageNames returns a new tags resolver
func NewFakeResolverDifferentImageNames() tags.Resolver {
	return &FakeResolver{}
}

// This fake resolver will allways give the same image_name, no matter the container ID

// FakeMonoResolver represents a fake mono resolver
type FakeMonoResolver struct {
}

// Start the resolver
func (fmr *FakeMonoResolver) Start(_ context.Context) error {
	return nil
}

// Stop the resolver
func (fmr *FakeMonoResolver) Stop() error {
	return nil
}

// Resolve returns the tags for the given id
func (fmr *FakeMonoResolver) Resolve(containerID string) []string {
	return []string{"container_id:" + containerID, "image_name:fake_ubuntu", "image_tag:latest"}
}

// ResolveWithErr returns the tags for the given id
func (fmr *FakeMonoResolver) ResolveWithErr(id string) ([]string, error) {
	return fmr.Resolve(id), nil
}

// GetValue return the tag value for the given id and tag name
func (fmr *FakeMonoResolver) GetValue(id string, tag string) string {
	return utils.GetTagValue(tag, fmr.Resolve(id))
}

// NewFakeMonoResolver returns a new tags resolver
func NewFakeMonoResolver() tags.Resolver {
	return &FakeMonoResolver{}
}

// This fake resolver will let us specify the next containerID to be resolved

// FakeManualResolver represents a fake manual resolver
type FakeManualResolver struct {
	sync.Mutex
	containerToSelector map[string]*cgroupModel.WorkloadSelector
	cpt                 int
	nextSelectors       []*cgroupModel.WorkloadSelector
}

// Start the resolver
func (fmr *FakeManualResolver) Start(_ context.Context) error {
	return nil
}

// Stop the resolver
func (fmr *FakeManualResolver) Stop() error {
	return nil
}

// SpecifyNextSelector specifies the next image name and tag to be resolved
func (fmr *FakeManualResolver) SpecifyNextSelector(selector *cgroupModel.WorkloadSelector) {
	fmr.Lock()
	defer fmr.Unlock()
	fmr.nextSelectors = append(fmr.nextSelectors, selector)
}

// GetContainerSelector returns the container selector
func (fmr *FakeManualResolver) GetContainerSelector(containerID string) *cgroupModel.WorkloadSelector {
	fmr.Lock()
	defer fmr.Unlock()
	if selector, found := fmr.containerToSelector[containerID]; found {
		return selector
	}
	return nil
}

// Resolve returns the tags for the given id
func (fmr *FakeManualResolver) Resolve(containerID string) []string {
	fmr.Lock()
	defer fmr.Unlock()

	// first, use cache if any
	selector, alreadyResolved := fmr.containerToSelector[containerID]
	if alreadyResolved {
		return []string{"container_id:" + containerID, "image_name:" + selector.Image, "image_tag:" + selector.Tag}
	}

	// if no cache and there is a pending list, use it
	if len(fmr.nextSelectors) > 0 {
		selector = fmr.nextSelectors[0]
		fmr.nextSelectors = fmr.nextSelectors[1:]
		fmr.containerToSelector[containerID] = selector
		return []string{"container_id:" + containerID, "image_name:" + selector.Image, "image_tag:" + selector.Tag}
	}

	// otherwise generate a new selector
	fmr.cpt++
	selector = &cgroupModel.WorkloadSelector{
		Image: fmt.Sprintf("fake_name_%d", fmr.cpt),
		Tag:   "fake_tag",
	}
	fmr.containerToSelector[containerID] = selector
	return []string{"container_id:" + containerID, "image_name:" + selector.Image, "image_tag:" + selector.Tag}
}

// ResolveWithErr returns the tags for the given id
func (fmr *FakeManualResolver) ResolveWithErr(id string) ([]string, error) {
	return fmr.Resolve(id), nil
}

// GetValue return the tag value for the given id and tag name
func (fmr *FakeManualResolver) GetValue(id string, tag string) string {
	return utils.GetTagValue(tag, fmr.Resolve(id))
}

// NewFakeManualResolver returns a new tags resolver
func NewFakeManualResolver() *FakeManualResolver {
	return &FakeManualResolver{
		containerToSelector: make(map[string]*cgroupModel.WorkloadSelector),
	}
}
