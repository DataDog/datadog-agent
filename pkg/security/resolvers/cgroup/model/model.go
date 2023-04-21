// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package model

import (
	"errors"
	"fmt"
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// WorkloadSelector is a selector used to uniquely indentify the image of a workload
type WorkloadSelector struct {
	Image string
	Tag   string
}

// NewWorkloadSelector returns an initialized instance of a WorkloadSelector
func NewWorkloadSelector(image string, tag string) (WorkloadSelector, error) {
	if image == "" {
		return WorkloadSelector{}, errors.New("no image name provided")
	} else if tag == "" {
		tag = "latest"
	}
	return WorkloadSelector{
		Image: image,
		Tag:   tag,
	}, nil
}

// IsEmpty returns true if the selector is set
func (ws *WorkloadSelector) IsEmpty() bool {
	return len(ws.Tag) != 0 && len(ws.Image) != 0
}

// Match returns true if the input selector matches the current selector
func (ws *WorkloadSelector) Match(selector WorkloadSelector) bool {
	return ws.Image == selector.Image && ws.Tag == selector.Tag
}

// String returns a string representation of a workload selector
func (ws WorkloadSelector) String() string {
	return fmt.Sprintf("[image_name:%s image_tag:%s]", ws.Image, ws.Tag)
}

// CacheEntry cgroup resolver cache entry
type CacheEntry struct {
	model.ContainerContext
	sync.RWMutex
	Deleted          *atomic.Bool
	WorkloadSelector WorkloadSelector
	PIDs             *simplelru.LRU[uint32, int8]
}

// NewCacheEntry returns a new instance of a CacheEntry
func NewCacheEntry(id string, pids ...uint32) (*CacheEntry, error) {
	pidsLRU, err := simplelru.NewLRU[uint32, int8](1000, nil)
	if err != nil {
		return nil, err
	}

	newCGroup := CacheEntry{
		Deleted: atomic.NewBool(false),
		ContainerContext: model.ContainerContext{
			ID: id,
		},
		PIDs: pidsLRU,
	}

	for _, pid := range pids {
		newCGroup.PIDs.Add(pid, 0)
	}
	return &newCGroup, nil
}

// GetPIDs returns the list of root pids for the current workload
func (cgce *CacheEntry) GetPIDs() []uint32 {
	cgce.RLock()
	defer cgce.RUnlock()

	return cgce.PIDs.Keys()
}

// RemovePID removes the provided root pid from the list of pids
func (cgce *CacheEntry) RemovePID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	cgce.PIDs.Remove(pid)
}

// AddPID adds a pid to the list of pids
func (cgce *CacheEntry) AddPID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	cgce.PIDs.Add(pid, 0)
}

// SetTags sets the tags for the provided workload
func (cgce *CacheEntry) SetTags(tags []string) {
	cgce.Lock()
	defer cgce.Unlock()

	cgce.Tags = tags
	cgce.WorkloadSelector.Image = utils.GetTagValue("image_name", tags)
	cgce.WorkloadSelector.Tag = utils.GetTagValue("image_tag", tags)
	if len(cgce.WorkloadSelector.Image) != 0 && len(cgce.WorkloadSelector.Tag) == 0 {
		cgce.WorkloadSelector.Tag = "latest"
	}
}

// NeedsTagsResolution returns true if this workload is missing its tags
func (cgce *CacheEntry) NeedsTagsResolution() bool {
	return len(cgce.ID) != 0 && !cgce.WorkloadSelector.IsEmpty()
}
