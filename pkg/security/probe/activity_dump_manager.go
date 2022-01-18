// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/cilium/ebpf"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ActivityDumpManager is used to manage ActivityDumps
type ActivityDumpManager struct {
	sync.RWMutex
	cleanupPeriod time.Duration
	probe         *Probe
	tracedPids    *ebpf.Map
	tracedComms   *ebpf.Map
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

	tracedComms, found, err := p.manager.GetMap("traced_comms")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("couldn't find traced_comms map")
	}

	return &ActivityDumpManager{
		probe:         p,
		statsdClient:  client,
		tracedPids:    tracedPIDs,
		tracedComms:   tracedComms,
		cleanupPeriod: p.config.ActivityDumpCleanupPeriod,
	}, nil
}

// DumpActivity handles an activity dump request
func (adm *ActivityDumpManager) DumpActivity(params *api.DumpActivityParams) (string, string, error) {
	adm.Lock()
	defer adm.Unlock()

	newDump, err := NewActivityDump(params, adm.tracedPids, adm.probe.resolvers, adm.probe.scrubber)
	if err != nil {
		return "", "", err
	}

	adm.activeDumps = append(adm.activeDumps, newDump)

	// push comm to kernel space
	if len(params.Comm) > 0 {
		commB := make([]byte, 16)
		copy(commB, params.Comm)
		value := uint32(1)
		err = adm.tracedComms.Put(commB, &value)
		if err != nil {
			seclog.Debugf("couldn't insert activity dump filter comm(%s): %v", params.Comm, err)
		}
	}

	// loop through the process cache entry tree and push traced pids if necessary
	adm.probe.resolvers.ProcessResolver.Walk(adm.SearchTracedProcessCacheEntryCallback)

	// Snapshot the processes in each activity dump
	for _, ad := range adm.activeDumps {
		_ = ad.Snapshot()
	}

	seclog.Infof("profiling started for %s", newDump.GetSelectorStr())

	return newDump.OutputFile, newDump.GraphFile, nil
}

// ListActivityDumps returns the list of active activity dumps
func (adm *ActivityDumpManager) ListActivityDumps(params *api.ListActivityDumpsParams) []string {
	adm.Lock()
	defer adm.Unlock()

	var activeDumps []string
	for _, d := range adm.activeDumps {
		activeDumps = append(activeDumps, d.GetSelectorStr())
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
		if (d.TagsListMatches(params.Tags) && inputDump.TagsListMatches(d.Tags)) || d.CommMatches(params.Comm) {
			d.Done()
			seclog.Infof("profiling stopped for %s", d.GetSelectorStr())
			toDelete = i

			// push comm to kernel space
			if len(d.Comm) > 0 {
				commB := make([]byte, 16)
				copy(commB, d.Comm)
				err := adm.tracedComms.Delete(commB)
				if err != nil {
					seclog.Debugf("couldn't delete activity dump filter comm(%s): %v", d.Comm, err)
				}
			}
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
		d.Insert(event)
	}
}

// SearchTracedProcessCacheEntryCallback inserts traced pids if necessary
func (adm *ActivityDumpManager) SearchTracedProcessCacheEntryCallback(entry *model.ProcessCacheEntry) {
	// compute the list of ancestors, we need to start inserting them from the root
	ancestors := []*model.ProcessCacheEntry{entry}
	parent := entry.GetNextAncestorNoFork()
	for parent != nil {
		ancestors = append([]*model.ProcessCacheEntry{parent}, ancestors...)
		parent = parent.GetNextAncestorNoFork()
	}

	for _, d := range adm.activeDumps {
		for _, parent = range ancestors {
			if node := d.FindOrCreateProcessActivityNode(parent); node != nil {
				_ = d.tracedPIDs.Put(node.Process.Pid, uint64(0))
			}
		}
	}
	return
}

var profileTmpl = `---
name: {{ .Name }}
selector:
  - {{ .Selector }}

rules:{{ range .Rules }}
  - id: {{ .ID }}
    expression: {{ .Expression }}
{{ end }}
`

// GenerateProfile returns a profile generated from the provided activity dump
func (adm *ActivityDumpManager) GenerateProfile(params *api.GenerateProfileParams) (string, error) {
	// open and parse activity dump file
	f, err := os.Open(params.ActivityDumpFile)
	if err != nil {
		return "", errors.Wrap(err, "couldn't open activity dump file")
	}

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return "", errors.Wrap(err, "couldn't read activity dump file")
	}

	var dump ActivityDump
	err = json.Unmarshal(data, &dump)
	if err != nil {
		return "", errors.Wrap(err, "couldn't parse activity dump file")
	}

	// create profile output file
	var profile *os.File
	profile, err = ioutil.TempFile("/tmp", "profile-")
	if err != nil {
		return "", errors.Wrap(err, "couldn't create profile file")
	}

	if err = os.Chmod(profile.Name(), 0400); err != nil {
		return "", errors.Wrap(err, "couldn't change the mode of the profile file")
	}

	t := template.Must(template.New("tmpl").Parse(profileTmpl))
	err = t.Execute(profile, dump.GenerateProfileData())
	if err != nil {
		return "", errors.Wrap(err, "couldn't generate profile")
	}

	return profile.Name(), nil
}

// SendStats sends the activity dump manager stats
func (adm *ActivityDumpManager) SendStats() error {
	adm.Lock()
	defer adm.Unlock()

	for _, dump := range adm.activeDumps {
		if err := dump.SendStats(adm.probe.statsdClient); err != nil {
			return errors.Wrapf(err, "couldn't send metrics for %s", dump.GetSelectorStr())
		}
	}
	return nil
}
