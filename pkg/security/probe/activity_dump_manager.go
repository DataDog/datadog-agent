// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe/activity_dump"
	"github.com/cilium/ebpf"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/api"
	seclog "github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func getTracedCgroupsCount(p *Probe) uint64 {
	return uint64(p.config.ActivityDumpTracedCgroupsCount)
}

func getCgroupDumpTimeout(p *Probe) uint64 {
	return uint64(p.config.ActivityDumpCgroupDumpTimeout.Nanoseconds())
}

// ActivityDumpManager is used to manage ActivityDumps
type ActivityDumpManager struct {
	sync.RWMutex
	probe               *Probe
	tracedPIDsMap       *ebpf.Map
	tracedCommsMap      *ebpf.Map
	tracedEventTypesMap *ebpf.Map
	tracedCgroupsMap    *ebpf.Map
	cgroupWaitListMap   *ebpf.Map

	activeDumps   []*ActivityDump
	snapshotQueue chan *ActivityDump
	storage       *ActivityDumpStorageManager
}

// Start runs the ActivityDumpManager
func (adm *ActivityDumpManager) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ticker := time.NewTicker(adm.probe.config.ActivityDumpCleanupPeriod)
	defer ticker.Stop()

	tagsTicker := time.NewTicker(adm.probe.config.ActivityDumpTagsResolutionPeriod)
	defer tagsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			adm.cleanup()
		case <-tagsTicker.C:
			adm.resolveTags()
		case dump := <-adm.snapshotQueue:
			if err := dump.Snapshot(); err != nil {
				seclog.Errorf("couldn't snapshot [%s]: %v", dump.GetSelectorStr(), err)
			}
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
			d.Stop()
			seclog.Infof("tracing stopped for [%s]", d.GetSelectorStr())

			// prepend dump ids to delete
			toDelete = append([]int{i}, toDelete...)

			// persist dump
			if err := adm.storage.Persist(d); err != nil {
				seclog.Errorf("couldn't persist dump [%s]: %v", d.GetSelectorStr(), err)
			}

			// release dump
			d.Release()
		}
	}

	for _, i := range toDelete {
		adm.activeDumps = append(adm.activeDumps[:i], adm.activeDumps[i+1:]...)
	}
}

// resolveTags resolves activity dump container tags when they are missing
func (adm *ActivityDumpManager) resolveTags() {
	adm.Lock()
	defer adm.Unlock()

	var err error
	for _, dump := range adm.activeDumps {
		err = dump.ResolveTags()
		if err != nil {
			seclog.Warnf("couldn't resolve activity dump tags (will try again later): %v", err)
		}
	}
}

// NewActivityDumpManager returns a new ActivityDumpManager instance
func NewActivityDumpManager(p *Probe) (*ActivityDumpManager, error) {
	tracedPIDs, found, err := p.manager.GetMap("traced_pids")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find traced_pids map")
	}

	tracedComms, found, err := p.manager.GetMap("traced_comms")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find traced_comms map")
	}

	cgroupWaitList, found, err := p.manager.GetMap("cgroup_wait_list")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find cgroup_wait_list map")
	}

	tracedEventTypesMap, found, err := p.manager.GetMap("traced_event_types")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find traced_event_types map")
	}

	// init traced event types
	isTraced := uint64(1)
	for _, evtType := range p.config.ActivityDumpTracedEventTypes {
		err = tracedEventTypesMap.Put(evtType, isTraced)
		if err != nil {
			return nil, fmt.Errorf("failed to insert traced event type: ")
		}
	}

	tracedCgroupsMap, found, err := p.manager.GetMap("traced_cgroups")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find traced_cgroups map")
	}

	storageManager, err := NewActivityDumpStorageManager(p)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate the activity dump storage manager: %w", err)
	}

	return &ActivityDumpManager{
		probe:               p,
		tracedPIDsMap:       tracedPIDs,
		tracedCommsMap:      tracedComms,
		tracedEventTypesMap: tracedEventTypesMap,
		tracedCgroupsMap:    tracedCgroupsMap,
		cgroupWaitListMap:   cgroupWaitList,
		snapshotQueue:       make(chan *ActivityDump, 100),
		storage:             storageManager,
	}, nil
}

// insertActivityDump inserts an activity dump in the list of activity dumps handled by the manager
func (adm *ActivityDumpManager) insertActivityDump(newDump *ActivityDump) error {
	// sanity checks
	if len(newDump.ContainerID) > 0 {
		// check if the provided container ID is new
		for _, dump := range adm.activeDumps {
			if dump.ContainerID == newDump.ContainerID {
				// an activity dump is already active for this container ID, ignore
				return nil
			}
		}
	}

	if len(newDump.Comm) > 0 {
		// check if the provided comm is new
		for _, dump := range adm.activeDumps {
			if dump.Comm == newDump.Comm {
				return fmt.Errorf("an activity dump is already active for the provided comm")
			}
		}
	}

	// dump will be added, push kernel space filters
	if len(newDump.ContainerID) > 0 {
		// put this container ID on the wait list so that we don't snapshot it again before a while
		containerIDB := make([]byte, model.ContainerIDLen)
		copy(containerIDB, newDump.ContainerID)
		waitListTimeout := time.Now().Add(time.Duration(adm.probe.config.ActivityDumpCgroupWaitListSize) * adm.probe.config.ActivityDumpCgroupDumpTimeout)
		waitListTimeoutRaw := adm.probe.resolvers.TimeResolver.ComputeMonotonicTimestamp(waitListTimeout)
		err := adm.cgroupWaitListMap.Put(containerIDB, waitListTimeoutRaw)
		if err != nil {
			seclog.Debugf("couldn't insert container ID %s to cgroup_wait_list: %v", newDump.ContainerID, err)
		}
	}

	if len(newDump.Comm) > 0 {
		commB := make([]byte, 16)
		copy(commB, newDump.Comm)
		value := newDump.getTimeoutRawTimestamp()
		err := adm.tracedCommsMap.Put(commB, &value)
		if err != nil {
			seclog.Debugf("couldn't insert activity dump filter comm(%s): %v", newDump.Comm, err)
		}
	}

	// loop through the process cache entry tree and push traced pids if necessary
	adm.probe.resolvers.ProcessResolver.Walk(adm.SearchTracedProcessCacheEntryCallback(newDump))

	// Delay the activity dump snapshot to reduce the overhead on the main goroutine
	select {
	case adm.snapshotQueue <- newDump:
	default:
	}

	// append activity dump to the list of active dumps
	newDump.SetState(Running)
	adm.activeDumps = append(adm.activeDumps, newDump)
	return nil
}

// HandleCgroupTracingEvent handles a cgroup tracing event
func (adm *ActivityDumpManager) HandleCgroupTracingEvent(event *model.CgroupTracingEvent) {
	adm.Lock()
	defer adm.Unlock()

	if len(event.ContainerContext.ID) == 0 {
		seclog.Errorf("received a cgroup tracing event with an empty container ID")
		return
	}
	newDump := NewActivityDump(adm, func(ad *ActivityDump) {
		ad.ContainerID = event.ContainerContext.ID
		ad.Timeout = adm.probe.resolvers.TimeResolver.ResolveMonotonicTimestamp(event.TimeoutRaw).Sub(time.Now())
		ad.DifferentiateArgs = adm.probe.config.ActivityDumpCgroupDifferentiateGraphs
	})

	// add local storage requests
	for _, format := range adm.probe.config.ActivityDumpLocalStorageFormats {
		newDump.AddStorageRequest(activity_dump.NewStorageRequest(
			activity_dump.LocalStorage,
			format,
			adm.probe.config.ActivityDumpLocalStorageCompression,
			adm.probe.config.ActivityDumpLocalStorageDirectory,
		))
	}

	// add remote storage requests
	for _, format := range adm.probe.config.ActivityDumpRemoteStorageFormats {
		newDump.AddStorageRequest(activity_dump.NewStorageRequest(
			activity_dump.RemoteStorage,
			format,
			adm.probe.config.ActivityDumpRemoteStorageCompression,
			"",
		))
	}

	if err := adm.insertActivityDump(newDump); err != nil {
		seclog.Errorf("couldn't start tracing [%s]: %v", newDump.GetSelectorStr(), err)
		return
	}
	seclog.Infof("tracing started for [%s]", newDump.GetSelectorStr())
}

// DumpActivity handles an activity dump request
func (adm *ActivityDumpManager) DumpActivity(params *api.DumpActivityParams) (*api.SecurityActivityDumpMessage, error) {
	adm.Lock()
	defer adm.Unlock()

	newDump := NewActivityDump(adm, func(ad *ActivityDump) {
		ad.Comm = params.GetComm()
		ad.Timeout = time.Duration(params.Timeout) * time.Minute
		ad.DifferentiateArgs = params.GetDifferentiateArgs()
	})

	// add local storage requests
	storageRequests, err := activity_dump.ParseStorageRequests(params.GetStorage())
	if err != nil {
		errMsg := fmt.Errorf("couldn't start tracing [%s]: %v", newDump.GetSelectorStr(), err)
		return &api.SecurityActivityDumpMessage{Error: errMsg.Error()}, errMsg
	}
	for _, request := range storageRequests {
		newDump.AddStorageRequest(request)
	}

	if err = adm.insertActivityDump(newDump); err != nil {
		errMsg := fmt.Errorf("couldn't start tracing [%s]: %v", newDump.GetSelectorStr(), err)
		return &api.SecurityActivityDumpMessage{Error: errMsg.Error()}, errMsg
	}
	seclog.Infof("tracing started for [%s]", newDump.GetSelectorStr())

	return newDump.ToSecurityActivityDumpMessage(), nil
}

// ListActivityDumps returns the list of active activity dumps
func (adm *ActivityDumpManager) ListActivityDumps(params *api.ListActivityDumpsParams) (*api.SecurityActivityDumpListMessage, error) {
	adm.Lock()
	defer adm.Unlock()

	var activeDumps []*api.SecurityActivityDumpMessage
	for _, d := range adm.activeDumps {
		activeDumps = append(activeDumps, d.ToSecurityActivityDumpMessage())
	}
	return &api.SecurityActivityDumpListMessage{
		Dumps: activeDumps,
	}, nil
}

// StopActivityDump stops an active activity dump
func (adm *ActivityDumpManager) StopActivityDump(params *api.StopActivityDumpParams) (*api.SecurityActivityDumpStoppedMessage, error) {
	adm.Lock()
	defer adm.Unlock()

	toDelete := -1
	for i, d := range adm.activeDumps {
		if d.commMatches(params.GetComm()) {
			d.Stop()
			seclog.Infof("tracing stopped for [%s]", d.GetSelectorStr())
			toDelete = i

			// persist now
			if err := adm.storage.Persist(d); err != nil {
				seclog.Errorf("couldn't persist [%s]: %v", d.GetSelectorStr(), err)
			}

			// release activity dump resources
			d.Release()
			break
		}
	}
	if toDelete >= 0 {
		adm.activeDumps = append(adm.activeDumps[:toDelete], adm.activeDumps[toDelete+1:]...)
		return &api.SecurityActivityDumpStoppedMessage{}, nil
	}
	errMsg := errors.Errorf("the activity dump manager does not contain any ActivityDump with the following comm: %s", params.GetComm())
	return &api.SecurityActivityDumpStoppedMessage{Error: errMsg.Error()}, errMsg
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
func (adm *ActivityDumpManager) SearchTracedProcessCacheEntryCallback(ad *ActivityDump) func(entry *model.ProcessCacheEntry) {
	return func(entry *model.ProcessCacheEntry) {
		ad.Lock()
		defer ad.Unlock()

		// compute the list of ancestors, we need to start inserting them from the root
		ancestors := []*model.ProcessCacheEntry{entry}
		parent := entry.GetNextAncestorNoFork()
		for parent != nil {
			ancestors = append([]*model.ProcessCacheEntry{parent}, ancestors...)
			parent = parent.GetNextAncestorNoFork()
		}

		for _, parent = range ancestors {
			if node := ad.findOrCreateProcessActivityNode(parent, Snapshot); node != nil {
				ad.updateTracedPidTimeout(node.Process.Pid)
			}
		}
	}
}

// TranscodingRequest executes the requested transcoding operation
func (adm *ActivityDumpManager) TranscodingRequest(params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	adm.Lock()
	defer adm.Unlock()
	var dump ActivityDump

	// open and parse input file
	if err := dump.Decode(params.GetActivityDumpFile()); err != nil {
		errMsg := errors.Errorf("couldn't parse input file %s: %v", params.GetActivityDumpFile(), err)
		return &api.TranscodingRequestMessage{Error: errMsg.Error()}, errMsg
	}

	// add transcoding requests
	storageRequests, err := activity_dump.ParseStorageRequests(params.GetStorage())
	if err != nil {
		errMsg := fmt.Errorf("couldn't parse transcoding request for [%s]: %v", dump.GetSelectorStr(), err)
		return &api.TranscodingRequestMessage{Error: errMsg.Error()}, errMsg
	}
	for _, request := range storageRequests {
		dump.AddStorageRequest(request)
	}

	// persist to execute transcoding request
	if err = adm.storage.Persist(&dump); err != nil {
		seclog.Errorf("couldn't persist [%s]: %v", dump.GetSelectorStr(), err)
	}

	return dump.ToTranscodingRequestMessage(), nil
}

// SendStats sends the activity dump manager stats
func (adm *ActivityDumpManager) SendStats() error {
	adm.Lock()
	defer adm.Unlock()

	for _, dump := range adm.activeDumps {
		if err := dump.SendStats(); err != nil {
			return errors.Wrapf(err, "couldn't send metrics for [%s]", dump.GetSelectorStr())
		}
	}

	activeDumps := float64(len(adm.activeDumps))
	if err := adm.probe.statsdClient.Gauge(metrics.MetricActivityDumpActiveDumps, activeDumps, []string{}, 1.0); err != nil {
		seclog.Errorf("couldn't send MetricActivityDumpActiveDumps metric: %v", err)
	}
	return nil
}

// snapshotTracedCgroups snapshots the kernel space map of cgroups
func (adm *ActivityDumpManager) snapshotTracedCgroups() {
	var err error
	var event model.CgroupTracingEvent
	containerIDB := make([]byte, model.ContainerIDLen)
	iterator := adm.tracedCgroupsMap.Iterate()

	for iterator.Next(containerIDB, &event.TimeoutRaw) {
		if _, err = event.ContainerContext.UnmarshalBinary(containerIDB[:]); err != nil {
			continue
		}

		adm.HandleCgroupTracingEvent(&event)
	}

	if err = iterator.Err(); err != nil {
		seclog.Errorf("couldn't iterate over the map traced_cgroups: %v", err)
	}
}
