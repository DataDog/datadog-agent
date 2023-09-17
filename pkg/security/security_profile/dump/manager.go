// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dump holds dump related files
package dump

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// ActivityDumpHandler represents an handler for the activity dumps sent by the probe
type ActivityDumpHandler interface {
	HandleActivityDump(dump *api.ActivityDumpStreamMessage)
}

// SecurityProfileManager is a generic interface used to communicate with the Security Profile manager
type SecurityProfileManager interface {
	FetchSilentWorkloads() map[cgroupModel.WorkloadSelector][]*cgroupModel.CacheEntry
}

// ActivityDumpManager is used to manage ActivityDumps
type ActivityDumpManager struct {
	sync.RWMutex
	config                 *config.Config
	statsdClient           statsd.ClientInterface
	emptyDropped           *atomic.Uint64
	dropMaxDumpReached     *atomic.Uint64
	newEvent               func() *model.Event
	resolvers              *resolvers.Resolvers
	kernelVersion          *kernel.Version
	manager                *manager.Manager
	dumpHandler            ActivityDumpHandler
	securityProfileManager SecurityProfileManager

	tracedPIDsMap          *ebpf.Map
	tracedCommsMap         *ebpf.Map
	tracedCgroupsMap       *ebpf.Map
	cgroupWaitList         *ebpf.Map
	activityDumpsConfigMap *ebpf.Map
	ignoreFromSnapshot     map[string]bool

	dumpLimiter *lru.Cache[cgroupModel.WorkloadSelector, *atomic.Uint64]

	activeDumps         []*ActivityDump
	snapshotQueue       chan *ActivityDump
	storage             *ActivityDumpStorageManager
	loadController      *ActivityDumpLoadController
	contextTags         []string
	hostname            string
	lastStoppedDumpTime time.Time
	pathsReducer        *activity_tree.PathsReducer
}

// Start runs the ActivityDumpManager
func (adm *ActivityDumpManager) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ticker := time.NewTicker(adm.config.RuntimeSecurity.ActivityDumpCleanupPeriod)
	defer ticker.Stop()

	tagsTicker := time.NewTicker(adm.config.RuntimeSecurity.ActivityDumpTagsResolutionPeriod)
	defer tagsTicker.Stop()

	loadControlTicker := time.NewTicker(adm.config.RuntimeSecurity.ActivityDumpLoadControlPeriod)
	defer loadControlTicker.Stop()

	silentWorkloadsTicker := time.NewTicker(adm.config.RuntimeSecurity.ActivityDumpSilentWorkloadsTicker)
	defer silentWorkloadsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			adm.cleanup()
		case <-tagsTicker.C:
			adm.resolveTags()
		case <-loadControlTicker.C:
			adm.triggerLoadController()
		case ad := <-adm.snapshotQueue:
			if err := ad.Snapshot(); err != nil {
				seclog.Errorf("couldn't snapshot [%s]: %v", ad.GetSelectorStr(), err)
			}
		case <-silentWorkloadsTicker.C:
			adm.handleSilentWorkloads()
		}
	}
}

// cleanup
func (adm *ActivityDumpManager) cleanup() {
	// fetch expired dumps
	dumps := adm.getExpiredDumps()

	for _, ad := range dumps {
		ad.Finalize(true)
		seclog.Infof("tracing stopped for [%s]", ad.GetSelectorStr())

		// persist dump if not empty
		if !ad.IsEmpty() {
			if err := adm.storage.Persist(ad); err != nil {
				seclog.Errorf("couldn't persist dump [%s]: %v", ad.GetSelectorStr(), err)
			}
		} else {
			adm.emptyDropped.Inc()
		}

		// remove from the map of ignored dumps
		adm.Lock()
		delete(adm.ignoreFromSnapshot, ad.Metadata.ContainerID)
		adm.Unlock()
	}

	// cleanup cgroup_wait_list map
	iterator := adm.cgroupWaitList.Iterate()
	containerIDB := make([]byte, model.ContainerIDLen)
	var timestamp uint64

	for iterator.Next(&containerIDB, &timestamp) {
		if time.Now().After(adm.resolvers.TimeResolver.ResolveMonotonicTimestamp(timestamp)) {
			if err := adm.cgroupWaitList.Delete(&containerIDB); err != nil {
				seclog.Errorf("couldn't delete cgroup_wait_list entry for (%s): %v", string(containerIDB), err)
			}
		}
	}

}

// getExpiredDumps returns the list of dumps that have timed out
func (adm *ActivityDumpManager) getExpiredDumps() []*ActivityDump {
	adm.Lock()
	defer adm.Unlock()

	var dumps []*ActivityDump
	var toDelete []int
	for i, ad := range adm.activeDumps {
		if time.Now().After(ad.Metadata.End) {
			toDelete = append([]int{i}, toDelete...)
			dumps = append(dumps, ad)
			adm.ignoreFromSnapshot[ad.Metadata.ContainerID] = true
		}
	}
	for _, i := range toDelete {
		adm.activeDumps = append(adm.activeDumps[:i], adm.activeDumps[i+1:]...)
	}
	return dumps
}

// resolveTags resolves activity dump container tags when they are missing
func (adm *ActivityDumpManager) resolveTags() {
	// fetch the list of dumps and release the manager as soon as possible
	adm.Lock()
	dumps := make([]*ActivityDump, len(adm.activeDumps))
	copy(dumps, adm.activeDumps)
	adm.Unlock()

	var err error
	for _, ad := range dumps {
		err = ad.ResolveTags()
		if err != nil {
			seclog.Warnf("couldn't resolve activity dump tags (will try again later): %v", err)
		}

		if !ad.countedByLimiter {
			// check if we should discard this dump based on the manager dump limiter
			selector := ad.GetWorkloadSelector()
			if selector == nil {
				// wait for the tags
				continue
			}

			counter, ok := adm.dumpLimiter.Get(*selector)
			if !ok {
				counter = atomic.NewUint64(0)
				adm.dumpLimiter.Add(*selector, counter)
			}

			if counter.Load() >= uint64(ad.adm.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload) {
				ad.Finalize(true)
				adm.RemoveDump(ad)
				adm.dropMaxDumpReached.Inc()
			} else {
				ad.countedByLimiter = true
				counter.Add(1)
			}
		}
	}
}

// AddActivityDumpHandler set the probe activity dump handler
func (adm *ActivityDumpManager) AddActivityDumpHandler(handler ActivityDumpHandler) {
	adm.dumpHandler = handler
}

// HandleActivityDump sends an activity dump to the backend
func (adm *ActivityDumpManager) HandleActivityDump(dump *api.ActivityDumpStreamMessage) {
	if adm.dumpHandler != nil {
		adm.dumpHandler.HandleActivityDump(dump)
	}
}

// NewActivityDumpManager returns a new ActivityDumpManager instance
func NewActivityDumpManager(config *config.Config, statsdClient statsd.ClientInterface, newEvent func() *model.Event, resolvers *resolvers.Resolvers,
	kernelVersion *kernel.Version, manager *manager.Manager) (*ActivityDumpManager, error) {
	tracedPIDs, err := managerhelper.Map(manager, "traced_pids")
	if err != nil {
		return nil, err
	}

	tracedComms, err := managerhelper.Map(manager, "traced_comms")
	if err != nil {
		return nil, err
	}

	tracedCgroupsMap, err := managerhelper.Map(manager, "traced_cgroups")
	if err != nil {
		return nil, err
	}

	activityDumpsConfigMap, err := managerhelper.Map(manager, "activity_dumps_config")
	if err != nil {
		return nil, err
	}

	cgroupWaitList, err := managerhelper.Map(manager, "cgroup_wait_list")
	if err != nil {
		return nil, err
	}

	limiter, err := lru.NewWithEvict(1024, func(workloadSelector cgroupModel.WorkloadSelector, count *atomic.Uint64) {
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't create dump limiter: %w", err)
	}

	adm := &ActivityDumpManager{
		config:                 config,
		statsdClient:           statsdClient,
		emptyDropped:           atomic.NewUint64(0),
		dropMaxDumpReached:     atomic.NewUint64(0),
		newEvent:               newEvent,
		resolvers:              resolvers,
		kernelVersion:          kernelVersion,
		manager:                manager,
		tracedPIDsMap:          tracedPIDs,
		tracedCommsMap:         tracedComms,
		tracedCgroupsMap:       tracedCgroupsMap,
		cgroupWaitList:         cgroupWaitList,
		activityDumpsConfigMap: activityDumpsConfigMap,
		snapshotQueue:          make(chan *ActivityDump, 100),
		ignoreFromSnapshot:     make(map[string]bool),
		dumpLimiter:            limiter,
		pathsReducer:           activity_tree.NewPathsReducer(),
	}

	adm.storage, err = NewActivityDumpStorageManager(config, statsdClient, adm)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate the activity dump storage manager: %w", err)
	}

	loadController, err := NewActivityDumpLoadController(adm)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate the activity dump load controller: %w", err)
	}
	if err = loadController.PushCurrentConfig(); err != nil {
		return nil, fmt.Errorf("failed to push load controller config settings to kernel space: %w", err)
	}
	adm.loadController = loadController

	adm.prepareContextTags()
	return adm, nil
}

func (adm *ActivityDumpManager) prepareContextTags() {
	// add hostname tag
	hostname, err := utils.GetHostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	adm.hostname = hostname
	adm.contextTags = append(adm.contextTags, fmt.Sprintf("host:%s", adm.hostname))

	// merge tags from config
	for _, tag := range configUtils.GetConfiguredTags(coreconfig.Datadog, true) {
		if strings.HasPrefix(tag, "host") {
			continue
		}
		adm.contextTags = append(adm.contextTags, tag)
	}

	// add source tag
	if len(utils.GetTagValue("source", adm.contextTags)) == 0 {
		adm.contextTags = append(adm.contextTags, fmt.Sprintf("source:%s", ActivityDumpSource))
	}
}

// insertActivityDump inserts an activity dump in the list of activity dumps handled by the manager
func (adm *ActivityDumpManager) insertActivityDump(newDump *ActivityDump) error {
	// sanity checks
	if len(newDump.Metadata.ContainerID) > 0 {
		// check if the provided container ID is new
		for _, ad := range adm.activeDumps {
			if ad.Metadata.ContainerID == newDump.Metadata.ContainerID {
				// an activity dump is already active for this container ID, ignore
				return nil
			}
		}
	}

	if len(newDump.Metadata.Comm) > 0 {
		// check if the provided comm is new
		for _, ad := range adm.activeDumps {
			if ad.Metadata.Comm == newDump.Metadata.Comm {
				return fmt.Errorf("an activity dump is already active for the provided comm")
			}
		}
	}

	// enable the new dump to start collecting events from kernel space
	if err := newDump.enable(); err != nil {
		return fmt.Errorf("couldn't insert new dump: %w", err)
	}

	// loop through the process cache entry tree and push traced pids if necessary
	adm.resolvers.ProcessResolver.Walk(adm.SearchTracedProcessCacheEntryCallback(newDump))

	// Delay the activity dump snapshot to reduce the overhead on the main goroutine
	select {
	case adm.snapshotQueue <- newDump:
	default:
	}

	// set the AD state now so that we can start inserting new events
	newDump.SetState(Running)

	// append activity dump to the list of active dumps
	adm.activeDumps = append(adm.activeDumps, newDump)

	seclog.Infof("tracing started for [%s]", newDump.GetSelectorStr())
	return nil
}

// handleDefaultDumpRequest starts dumping a new workload with the provided load configuration and the default dump configuration
func (adm *ActivityDumpManager) startDumpWithConfig(containerID string, cookie uint64, loadConfig model.ActivityDumpLoadConfig) {
	newDump := NewActivityDump(adm, func(ad *ActivityDump) {
		ad.Metadata.ContainerID = containerID
		ad.SetLoadConfig(cookie, loadConfig)

		if adm.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs {
			ad.Metadata.DifferentiateArgs = true
			ad.ActivityTree.DifferentiateArgs()
		}
	})

	// add local storage requests
	for _, format := range adm.config.RuntimeSecurity.ActivityDumpLocalStorageFormats {
		newDump.AddStorageRequest(config.NewStorageRequest(
			config.LocalStorage,
			format,
			adm.config.RuntimeSecurity.ActivityDumpLocalStorageCompression,
			adm.config.RuntimeSecurity.ActivityDumpLocalStorageDirectory,
		))
	}

	// add remote storage requests
	newDump.AddStorageRequest(config.NewStorageRequest(
		config.RemoteStorage,
		config.Protobuf,
		true, // force remote compression
		"",
	))

	if err := adm.insertActivityDump(newDump); err != nil {
		seclog.Errorf("couldn't start tracing [%s]: %v", newDump.GetSelectorStr(), err)
	}
}

// HandleCGroupTracingEvent handles a cgroup tracing event
func (adm *ActivityDumpManager) HandleCGroupTracingEvent(event *model.CgroupTracingEvent) {
	adm.Lock()
	defer adm.Unlock()

	if len(event.ContainerContext.ID) == 0 {
		seclog.Errorf("received a cgroup tracing event with an empty container ID")
		return
	}

	adm.startDumpWithConfig(event.ContainerContext.ID, event.ConfigCookie, event.Config)
}

// SetSecurityProfileManager sets the security profile manager
func (adm *ActivityDumpManager) SetSecurityProfileManager(manager SecurityProfileManager) {
	adm.Lock()
	defer adm.Unlock()
	adm.securityProfileManager = manager
}

// handleSilentWorkloads checks if we should start tracing one of the workloads from a profile without an activity tree of the Security Profile manager
func (adm *ActivityDumpManager) handleSilentWorkloads() {
	adm.Lock()
	defer adm.Unlock()

	if adm.securityProfileManager == nil {
		// the security profile manager hasn't been set yet
		return
	}

	// check if it's a good time to look for a silent workload, to do so, check if the last stopped dump was stopped more
	// than the configured amount of time ago
	if time.Since(adm.lastStoppedDumpTime) < adm.config.RuntimeSecurity.ActivityDumpSilentWorkloadsDelay {
		return
	}

	// if we're already at capacity leave now - this prevents an unnecessary lock on the security profile manager
	if len(adm.activeDumps) >= adm.config.RuntimeSecurity.ActivityDumpTracedCgroupsCount {
		return
	}

	// fetch silent workloads
workloadLoop:
	for selector, workloads := range adm.securityProfileManager.FetchSilentWorkloads() {
		if len(workloads) == 0 {
			// this profile is on its way out, ignore
			continue
		}

		if len(adm.activeDumps) >= adm.config.RuntimeSecurity.ActivityDumpTracedCgroupsCount {
			// we're at capacity, ignore for now
			break
		}

		// check if we already have an activity dump for this selector
		for _, ad := range adm.activeDumps {
			// the dump selector is resolved if it has been counted by the limiter
			if !ad.countedByLimiter {
				continue
			}

			if ad.selector.Match(selector) {
				// we already have an activity dump for this selector, ignore
				continue workloadLoop
			}
		}

		// if we're still here, we can start tracing this workload
		adm.startDumpWithConfig(workloads[0].ID, utils.NewCookie(), *adm.loadController.getDefaultLoadConfig())
	}
}

// DumpActivity handles an activity dump request
func (adm *ActivityDumpManager) DumpActivity(params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	adm.Lock()
	defer adm.Unlock()

	newDump := NewActivityDump(adm, func(ad *ActivityDump) {
		ad.Metadata.Comm = params.GetComm()
		ad.Metadata.ContainerID = params.GetContainerID()
		dumpDuration, _ := time.ParseDuration(params.Timeout)
		ad.SetTimeout(dumpDuration)

		if params.GetDifferentiateArgs() {
			ad.Metadata.DifferentiateArgs = true
			ad.ActivityTree.DifferentiateArgs()
		}
	})

	// add local storage requests
	storageRequests, err := config.ParseStorageRequests(params.GetStorage())
	if err != nil {
		errMsg := fmt.Errorf("couldn't start tracing [%s]: %v", newDump.GetSelectorStr(), err)
		return &api.ActivityDumpMessage{Error: errMsg.Error()}, errMsg
	}
	for _, request := range storageRequests {
		newDump.AddStorageRequest(request)
	}

	if err = adm.insertActivityDump(newDump); err != nil {
		errMsg := fmt.Errorf("couldn't start tracing [%s]: %v", newDump.GetSelectorStr(), err)
		return &api.ActivityDumpMessage{Error: errMsg.Error()}, errMsg
	}

	return newDump.ToSecurityActivityDumpMessage(), nil
}

// ListActivityDumps returns the list of active activity dumps
func (adm *ActivityDumpManager) ListActivityDumps(params *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	adm.Lock()
	defer adm.Unlock()

	var activeDumps []*api.ActivityDumpMessage
	for _, d := range adm.activeDumps {
		activeDumps = append(activeDumps, d.ToSecurityActivityDumpMessage())
	}
	return &api.ActivityDumpListMessage{
		Dumps: activeDumps,
	}, nil
}

// RemoveDump removes a dump
func (adm *ActivityDumpManager) RemoveDump(dump *ActivityDump) {
	adm.Lock()
	defer adm.Unlock()
	adm.removeDump(dump)
}

func (adm *ActivityDumpManager) removeDump(dump *ActivityDump) {
	toDelete := -1
	for i, d := range adm.activeDumps {
		if d.Name == dump.Name {
			toDelete = i
			break
		}
	}
	if toDelete >= 0 {
		adm.activeDumps = append(adm.activeDumps[:toDelete], adm.activeDumps[toDelete+1:]...)
	}
}

// StopActivityDump stops an active activity dump
func (adm *ActivityDumpManager) StopActivityDump(params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	adm.Lock()
	defer adm.Unlock()

	if params.GetName() == "" && params.GetContainerID() == "" && params.GetComm() == "" {
		errMsg := fmt.Errorf("you must specify one selector between name, containerID and comm")
		return &api.ActivityDumpStopMessage{Error: errMsg.Error()}, errMsg
	}

	toDelete := -1
	for i, d := range adm.activeDumps {
		if (params.GetName() != "" && d.nameMatches(params.GetName())) ||
			(params.GetContainerID() != "" && d.containerIDMatches(params.GetContainerID())) ||
			(params.GetComm() != "" && d.commMatches(params.GetComm())) {
			d.Finalize(true)
			seclog.Infof("tracing stopped for [%s]", d.GetSelectorStr())
			toDelete = i

			// persist dump if not empty
			if !d.IsEmpty() {
				if err := adm.storage.Persist(d); err != nil {
					seclog.Errorf("couldn't persist dump [%s]: %v", d.GetSelectorStr(), err)
				}
			} else {
				adm.emptyDropped.Inc()
			}
			break
		}
	}
	if toDelete >= 0 {
		adm.activeDumps = append(adm.activeDumps[:toDelete], adm.activeDumps[toDelete+1:]...)
		return &api.ActivityDumpStopMessage{}, nil
	}
	var errMsg error
	if params.GetName() != "" {
		errMsg = fmt.Errorf("the activity dump manager does not contain any ActivityDump with the following name: %s", params.GetName())
	} else if params.GetContainerID() != "" {
		errMsg = fmt.Errorf("the activity dump manager does not contain any ActivityDump with the following containerID: %s", params.GetContainerID())
	} else /* if params.GetComm() != "" */ {
		errMsg = fmt.Errorf("the activity dump manager does not contain any ActivityDump with the following comm: %s", params.GetComm())
	}
	return &api.ActivityDumpStopMessage{Error: errMsg.Error()}, errMsg
}

// ProcessEvent processes a new event and insert it in an activity dump if applicable
func (adm *ActivityDumpManager) ProcessEvent(event *model.Event) {
	// ignore events with an error
	if event.Error != nil {
		return
	}

	// is this event sampled for activity dumps ?
	if !event.IsActivityDumpSample() {
		return
	}

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

		// check process lineage
		if !entry.HasCompleteLineage() {
			return
		}

		// compute the list of ancestors, we need to start inserting them from the root
		ancestors := []*model.ProcessCacheEntry{entry}
		parent := activity_tree.GetNextAncestorBinaryOrArgv0(&entry.ProcessContext)
		for parent != nil {
			ancestors = append([]*model.ProcessCacheEntry{parent}, ancestors...)
			parent = activity_tree.GetNextAncestorBinaryOrArgv0(&parent.ProcessContext)
		}

		for _, parent = range ancestors {
			_, _, _, err := ad.ActivityTree.CreateProcessNode(parent, nil, activity_tree.Snapshot, false, adm.resolvers)
			if err != nil {
				// if one of the parents wasn't inserted, leave now
				break
			}
		}
	}
}

// TranscodingRequest executes the requested transcoding operation
func (adm *ActivityDumpManager) TranscodingRequest(params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	adm.Lock()
	defer adm.Unlock()
	ad := NewActivityDump(adm)

	// open and parse input file
	if err := ad.Decode(params.GetActivityDumpFile()); err != nil {
		errMsg := fmt.Errorf("couldn't parse input file %s: %v", params.GetActivityDumpFile(), err)
		return &api.TranscodingRequestMessage{Error: errMsg.Error()}, errMsg
	}

	// add transcoding requests
	storageRequests, err := config.ParseStorageRequests(params.GetStorage())
	if err != nil {
		errMsg := fmt.Errorf("couldn't parse transcoding request for [%s]: %v", ad.GetSelectorStr(), err)
		return &api.TranscodingRequestMessage{Error: errMsg.Error()}, errMsg
	}
	for _, request := range storageRequests {
		ad.AddStorageRequest(request)
	}

	// persist to execute transcoding request
	if err = adm.storage.Persist(ad); err != nil {
		seclog.Errorf("couldn't persist [%s]: %v", ad.GetSelectorStr(), err)
	}

	return ad.ToTranscodingRequestMessage(), nil
}

// SendStats sends the activity dump manager stats
func (adm *ActivityDumpManager) SendStats() error {
	adm.Lock()
	defer adm.Unlock()

	for _, ad := range adm.activeDumps {
		if err := ad.SendStats(); err != nil {
			return fmt.Errorf("couldn't send metrics for [%s]: %w", ad.GetSelectorStr(), err)
		}
	}

	activeDumps := float64(len(adm.activeDumps))
	if err := adm.statsdClient.Gauge(metrics.MetricActivityDumpActiveDumps, activeDumps, []string{}, 1.0); err != nil {
		seclog.Errorf("couldn't send MetricActivityDumpActiveDumps metric: %v", err)
	}

	if value := adm.emptyDropped.Swap(0); value > 0 {
		if err := adm.statsdClient.Count(metrics.MetricActivityDumpEmptyDropped, int64(value), nil, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEmptyDropped, err)
		}
	}

	if value := adm.dropMaxDumpReached.Swap(0); value > 0 {
		if err := adm.statsdClient.Count(metrics.MetricActivityDumpDropMaxDumpReached, int64(value), nil, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpDropMaxDumpReached, err)
		}
	}

	return nil
}

// SnapshotTracedCgroups snapshots the kernel space map of cgroups
func (adm *ActivityDumpManager) SnapshotTracedCgroups() {
	var err error
	var event model.CgroupTracingEvent
	containerIDB := make([]byte, model.ContainerIDLen)
	iterator := adm.tracedCgroupsMap.Iterate()
	seclog.Infof("snapshotting traced_cgroups map")

	for iterator.Next(&containerIDB, &event.ConfigCookie) {
		adm.Lock()
		if adm.ignoreFromSnapshot[string(containerIDB)] {
			adm.Unlock()
			continue
		}
		adm.Unlock()

		if err = adm.activityDumpsConfigMap.Lookup(&event.ConfigCookie, &event.Config); err != nil {
			// this config doesn't exist anymore, remove expired entries
			seclog.Errorf("config not found for (%s): %v", string(containerIDB), err)
			_ = adm.tracedCgroupsMap.Delete(containerIDB)
			continue
		}

		if _, err = event.ContainerContext.UnmarshalBinary(containerIDB[:]); err != nil {
			seclog.Errorf("couldn't unmarshal container ID from traced_cgroups key: %v", err)
			// remove invalid entry
			_ = adm.tracedCgroupsMap.Delete(containerIDB)
			continue
		}

		adm.HandleCGroupTracingEvent(&event)
	}

	if err = iterator.Err(); err != nil {
		seclog.Errorf("couldn't iterate over the map traced_cgroups: %v", err)
	}
}

// AddContextTags adds context tags to the activity dump
func (adm *ActivityDumpManager) AddContextTags(ad *ActivityDump) {
	var tagName string
	var found bool

	dumpTagNames := make([]string, 0, len(ad.Tags))
	for _, tag := range ad.Tags {
		dumpTagNames = append(dumpTagNames, utils.GetTagName(tag))
	}

	for _, tag := range adm.contextTags {
		tagName = utils.GetTagName(tag)
		found = false

		for _, dumpTagName := range dumpTagNames {
			if tagName == dumpTagName {
				found = true
				break
			}
		}

		if !found {
			ad.Tags = append(ad.Tags, tag)
		}
	}
}

func (adm *ActivityDumpManager) triggerLoadController() {
	// fetch the list of overweight dump
	dumps := adm.getOverweightDumps()

	// handle overweight dumps
	for _, ad := range dumps {
		// stop the dump but do not release the cgroup
		ad.Finalize(false)
		seclog.Infof("tracing paused for [%s]", ad.GetSelectorStr())

		// persist dump if not empty
		if !ad.IsEmpty() {
			if err := adm.storage.Persist(ad); err != nil {
				seclog.Errorf("couldn't persist dump [%s]: %v", ad.GetSelectorStr(), err)
			}
		} else {
			adm.emptyDropped.Inc()
		}

		// restart a new dump for the same workload
		newDump := adm.loadController.NextPartialDump(ad)

		adm.Lock()
		if err := adm.insertActivityDump(newDump); err != nil {
			seclog.Errorf("couldn't resume tracing [%s]: %v", newDump.GetSelectorStr(), err)
			adm.Unlock()
			return
		}

		// remove container ID from the map of ignored container IDs for the snapshot
		delete(adm.ignoreFromSnapshot, ad.Metadata.ContainerID)
		adm.Unlock()
	}
}

// getOverweightDumps returns the list of dumps that crossed the config.ActivityDumpMaxDumpSize threshold
func (adm *ActivityDumpManager) getOverweightDumps() []*ActivityDump {
	adm.Lock()
	defer adm.Unlock()

	var dumps []*ActivityDump
	var toDelete []int
	for i, ad := range adm.activeDumps {
		dumpSize := ad.ComputeInMemorySize()

		// send dump size in memory metric
		if err := adm.statsdClient.Gauge(metrics.MetricActivityDumpActiveDumpSizeInMemory, float64(dumpSize), []string{fmt.Sprintf("dump_index:%d", i)}, 1); err != nil {
			seclog.Errorf("couldn't send %s metric: %v", metrics.MetricActivityDumpActiveDumpSizeInMemory, err)
		}

		if dumpSize >= int64(adm.config.RuntimeSecurity.ActivityDumpMaxDumpSize()) {
			toDelete = append([]int{i}, toDelete...)
			dumps = append(dumps, ad)
			adm.ignoreFromSnapshot[ad.Metadata.ContainerID] = true
		}
	}
	for _, i := range toDelete {
		adm.activeDumps = append(adm.activeDumps[:i], adm.activeDumps[i+1:]...)
	}
	return dumps
}

// FakeDumpOverweight fakes a dump stats to force triggering the load controller. For unitary tests purpose only.
func (adm *ActivityDumpManager) FakeDumpOverweight(name string) {
	adm.Lock()
	defer adm.Unlock()
	for _, ad := range adm.activeDumps {
		if ad.Name == name {
			ad.ActivityTree.Stats.ProcessNodes = int64(99999)
		}
	}
}

// StopDumpsWithSelector stops the active dumps for the given selector and prevent a workload with the provided selector from ever being dumped again
func (adm *ActivityDumpManager) StopDumpsWithSelector(selector cgroupModel.WorkloadSelector) {
	counter, ok := adm.dumpLimiter.Get(selector)
	if !ok {
		counter = atomic.NewUint64(uint64(adm.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload))
		adm.dumpLimiter.Add(selector, counter)
	} else {
		if counter.Load() < uint64(adm.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload) {
			seclog.Infof("activity dumps will no longer be generated for %s", selector.String())
			counter.Store(uint64(adm.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload))
		}
	}

	adm.Lock()
	activeDumps := make([]*ActivityDump, 0, len(adm.activeDumps))
	copy(activeDumps, adm.activeDumps)
	adm.Unlock()

	for _, ad := range activeDumps {
		ad.Lock()
		if adSelector := ad.GetWorkloadSelector(); adSelector != nil && adSelector.Match(selector) {
			ad.finalize(true)
			adm.RemoveDump(ad)
			adm.dropMaxDumpReached.Inc()
		}
		ad.Unlock()
	}
	return
}
