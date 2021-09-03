// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/cilium/ebpf"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/model"
)

// ActivityDumpManager is used to manage ActivityDumps
type ActivityDumpManager struct {
	sync.RWMutex
	cleanupPeriod time.Duration
	probe         *Probe
	tracedPids    *ebpf.Map
	statsdClient  *statsd.Client

	activeDumps []*ActivityDump
}

// Start runs the ActivityDumpManager
func (adm *ActivityDumpManager) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ticker := time.NewTicker(adm.cleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			adm.cleanup()
		}
	}
}

// cleanup
func (adm *ActivityDumpManager) cleanup() {
	adm.Lock()
	defer adm.Unlock()

	var toDelete []int

	for i, d := range adm.activeDumps {
		if time.Now().After(d.Start.Add(d.Timeout)) {
			d.Done()

			// prepend dump ids to delete
			toDelete = append([]int{i}, toDelete...)
		}
	}

	for _, i := range toDelete {
		adm.activeDumps = append(adm.activeDumps[:i], adm.activeDumps[i+1:]...)
	}
}

// NewActivityDumpManager returns a new ActivityDumpManager instance
func NewActivityDumpManager(p *Probe, client *statsd.Client) (*ActivityDumpManager, error) {
	tracedPIDs, found, err := p.manager.GetMap("traced_pids")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("couldn't find traced_pids map")
	}

	return &ActivityDumpManager{
		probe:         p,
		statsdClient:  client,
		tracedPids:    tracedPIDs,
		cleanupPeriod: p.config.ActivityDumpCleanupPeriod,
	}, nil
}

// DumpActivity handles an activity dump request
func (adm *ActivityDumpManager) DumpActivity(params *api.DumpActivityParams) (string, string, error) {
	adm.Lock()
	defer adm.Unlock()

	newDump, err := NewActivityDump(params.Tags, time.Duration(params.Timeout)*time.Minute, params.WithGraph, adm.tracedPids)
	if err != nil {
		return "", "", err
	}

	adm.activeDumps = append(adm.activeDumps, newDump)

	// loop through the process cache entry tree and push traced pids if necessary
	adm.probe.resolvers.ProcessResolver.Walk(adm.SearchTracedProcessCacheEntryCallback)

	return newDump.OutputFile, newDump.GraphFile, nil
}

// ListActivityDumps returns the list of active activity dumps
func (adm *ActivityDumpManager) ListActivityDumps(params *api.ListActivityDumpsParams) []string {
	adm.Lock()
	defer adm.Unlock()

	var activeDumps []string
	for _, d := range adm.activeDumps {
		activeDumps = append(activeDumps, strings.Join(d.Tags, ", "))
	}
	return activeDumps
}

// StopActivityDump stops an active activity dump
func (adm *ActivityDumpManager) StopActivityDump(params *api.StopActivityDumpParams) error {
	adm.Lock()
	defer adm.Unlock()

	toDelete := -1
	inputDump := ActivityDump{Tags: params.Tags}
	for i, d := range adm.activeDumps {
		if d.TagsListMatches(params.Tags) && inputDump.TagsListMatches(d.Tags) {
			d.Done()
			toDelete = i
			break
		}
	}
	if toDelete >= 0 {
		adm.activeDumps = append(adm.activeDumps[:toDelete], adm.activeDumps[toDelete+1:]...)
		return nil
	}
	return errors.Errorf("the activity dump manager does not contain any ActivityDump with the following set of tags: %s", strings.Join(params.Tags, ", "))
}

// ProcessEvent processes a new event and insert it in an activity dump if applicable
func (adm *ActivityDumpManager) ProcessEvent(event *Event) {
	adm.Lock()
	defer adm.Unlock()

	for _, d := range adm.activeDumps {
		_ = event.ResolveContainerTags(&event.ContainerContext)
		if d.TagsListMatches(event.GetTags()) {
			d.Insert(event)
		}
	}
}

// SearchTracedProcessCacheEntryCallback inserts traced pids if necessary
func (adm *ActivityDumpManager) SearchTracedProcessCacheEntryCallback(entry *model.ProcessCacheEntry) {
	for _, d := range adm.activeDumps {
		if d.TagsListMatches(adm.probe.resolvers.ResolvePCEContainerTags(entry)) {
			_ = d.tracedPIDs.Put(entry.Pid, uint64(0))
			return
		}
	}
	return
}
