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
		if cont.CgroupPath != "" {
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
func (cf *containerFilter) ContainerFilter(path, name string) (string, error) {
	if res, _ := cgroups.ContainerFilter(path, name); res != "" {
		return res, nil
	}
	cf.mutex.RLock()
	res, ok := cf.trie.Get(path)
	cf.mutex.RUnlock()
	if !ok || res == nil {
		return "", nil
	}
	return *res, nil
}
