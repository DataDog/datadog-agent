// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	"errors"
	"fmt"
	"sync"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	ErrNoImageProvided = errors.New("no image name provided") // ErrNoImageProvided is returned when no image name is provided
)

// WorkloadKey defines the key has uniquely defines a workload
type WorkloadKey string

// Match returns true if the input selector matches the current selector
func (wk *WorkloadKey) Match(selector WorkloadSelector) bool {
	return *wk == selector.Key()
}

// WorkloadSelector is a selector used to uniquely indentify the image of a workload
type WorkloadSelector struct {
	image string
	tag   string
}

// NewWorkloadSelector returns an initialized instance of a WorkloadSelector
func NewWorkloadSelector(image string, tag string) (WorkloadSelector, error) {
	if image == "" {
		return WorkloadSelector{}, ErrNoImageProvided
	} else if tag == "" {
		tag = "latest"
	}
	return WorkloadSelector{
		image: image,
		tag:   tag,
	}, nil
}

// Version returns the selector name
func (ws *WorkloadSelector) Name() string {
	return ws.image
}

// Version returns the selector version
func (ws *WorkloadSelector) Version() string {
	return ws.tag
}

// SetVersion sets the selector name
func (ws *WorkloadSelector) SetName(name string) {
	ws.image = name
}

// SetVersion sets the selector version
func (ws *WorkloadSelector) SetVersion(version string) {
	ws.tag = version
}

// Key returns the selector key
func (ws *WorkloadSelector) Key() WorkloadKey {
	return WorkloadKey(ws.image + ":" + ws.tag)
}

// IsReady returns true if the selector is ready
func (ws *WorkloadSelector) Clone() *WorkloadSelector {
	return &WorkloadSelector{
		image: ws.image,
		tag:   ws.tag,
	}
}

// IsReady returns true if the selector is ready
func (ws *WorkloadSelector) IsReady() bool {
	return len(ws.image) != 0
}

// Match returns true if the input selector matches the current selector
func (ws *WorkloadSelector) Match(selector WorkloadSelector) bool {
	if ws.tag == "*" || selector.tag == "*" {
		return ws.image == selector.image
	}
	return ws.image == selector.image && ws.tag == selector.tag
}

// String returns a string representation of a workload selector
func (ws WorkloadSelector) String() string {
	return fmt.Sprintf("[image_name:%s image_tag:%s]", ws.image, ws.tag)
}

// ToTags returns a string array representation of a workload selector
func (ws WorkloadSelector) ToTags() []string {
	return []string{
		"image_name:" + ws.Name(),
		"image_tag:" + ws.Version(),
	}
}

// CacheEntry cgroup resolver cache entry
type CacheEntry struct {
	model.ContainerContext
	sync.RWMutex
	Deleted          *atomic.Bool
	WorkloadSelector WorkloadSelector
	PIDs             map[uint32]int8
}

// NewCacheEntry returns a new instance of a CacheEntry
func NewCacheEntry(id string, pids ...uint32) (*CacheEntry, error) {
	newCGroup := CacheEntry{
		Deleted: atomic.NewBool(false),
		ContainerContext: model.ContainerContext{
			ID: id,
		},
		PIDs: make(map[uint32]int8, 10),
	}

	for _, pid := range pids {
		newCGroup.PIDs[pid] = 1
	}
	return &newCGroup, nil
}

// GetPIDs returns the list of pids for the current workload
func (cgce *CacheEntry) GetPIDs() []uint32 {
	cgce.RLock()
	defer cgce.RUnlock()

	pids := make([]uint32, len(cgce.PIDs))
	i := 0
	for k := range cgce.PIDs {
		pids[i] = k
		i++
	}

	return pids
}

// RemovePID removes the provided pid from the list of pids
func (cgce *CacheEntry) RemovePID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	delete(cgce.PIDs, pid)
}

// AddPID adds a pid to the list of pids
func (cgce *CacheEntry) AddPID(pid uint32) {
	cgce.Lock()
	defer cgce.Unlock()

	cgce.PIDs[pid] = 1
}

// SetTags sets the tags for the provided workload
func (cgce *CacheEntry) SetTags(tags []string) {
	cgce.Lock()
	defer cgce.Unlock()

	cgce.Tags = tags
	cgce.WorkloadSelector.image = utils.GetTagValue("image_name", tags)
	if cgce.WorkloadSelector.image == "" {
		cgce.WorkloadSelector.image = utils.GetTagValue("service", tags)
	}
	cgce.WorkloadSelector.tag = utils.GetTagValue("image_tag", tags)
	if cgce.WorkloadSelector.tag == "" {
		cgce.WorkloadSelector.tag = utils.GetTagValue("version", tags)
	}
	if len(cgce.WorkloadSelector.image) != 0 && len(cgce.WorkloadSelector.tag) == 0 {
		cgce.WorkloadSelector.tag = "latest"
	}
}

// GetWorkloadSelectorCopy returns a copy of the workload selector of this cgroup
func (cgce *CacheEntry) GetWorkloadSelectorCopy() *WorkloadSelector {
	cgce.RLock()
	defer cgce.RUnlock()

	return cgce.WorkloadSelector.Clone()
}

// NeedsTagsResolution returns true if this workload is missing its tags
func (cgce *CacheEntry) NeedsTagsResolution() bool {
	return len(cgce.ID) != 0 && !cgce.WorkloadSelector.IsReady()
}
