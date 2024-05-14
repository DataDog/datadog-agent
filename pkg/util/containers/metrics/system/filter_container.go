// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/trie"
)

// containerFilter is a filter that will first try to retrieve a container-id from the cgroup path
// using regex matching. If it fails, it will perform suffix-matching on the cgroup path to find a container-id.
// Containerd currently exposes either the full cgroup path or only a suffix, this is why we use a `trie` to store
// the metadata retrieved from workloadmeta.
type containerFilter struct {
	wlm workloadmeta.Component

	mutex sync.RWMutex
	trie  *trie.SuffixTrie[string]
}

// newContainerFilter returns a new container filter
func newContainerFilter(wlm workloadmeta.Component) *containerFilter {
	cf := &containerFilter{
		trie: trie.NewSuffixTrie[string](),
		wlm:  wlm,
	}
	return cf
}

func (cf *containerFilter) start() {
	if cf.wlm == nil {
		return
	}
	evBundle := cf.wlm.Subscribe("cid-mapper", workloadmeta.NormalPriority, workloadmeta.NewFilter(
		&workloadmeta.FilterParams{
			Kinds:     []workloadmeta.Kind{workloadmeta.KindContainer},
			Source:    workloadmeta.SourceAll,
			EventType: workloadmeta.EventTypeAll,
		},
	))
	for evs := range evBundle {
		evs.Acknowledge()
		cf.mutex.Lock()
		for _, ev := range evs.Events {
			cf.handleEvent(ev)
		}
		cf.mutex.Unlock()
	}
}

func (cf *containerFilter) handleEvent(ev workloadmeta.Event) {
	cont, ok := ev.Entity.(*workloadmeta.Container)
	if !ok {
		log.Errorf("unexpected event type: %T", ev)
		return
	}
	switch ev.Type {
	case workloadmeta.EventTypeSet:
		// As a memory optimization, we only store the container id in the trie
		// if the cgroup path is not already matched by the cgroup filter.
		if res, _ := cgroups.ContainerFilter("", cont.CgroupPath); res == "" {
			cid := cont.ID
			cf.trie.Insert(cont.CgroupPath, &cid)
		}
	case workloadmeta.EventTypeUnset:
		cf.trie.Delete(cont.CgroupPath)
	default:
		log.Errorf("unexpected event type: %v", ev.Type)
	}
}

// ContainerFilter returns a filter that will match cgroup folders containing a container id
func (cf *containerFilter) ContainerFilter(fullPath, name string) (string, error) {
	if res, _ := cgroups.ContainerFilter(fullPath, name); res != "" {
		return res, nil
	}
	cf.mutex.RLock()
	res, ok := cf.trie.Get(fullPath)
	cf.mutex.RUnlock()
	if !ok || res == nil {
		return "", nil
	}
	return *res, nil
}
