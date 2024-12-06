// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dump holds dump related files
package dump

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
	OnLocalStorageCleanup(files []string)
}

// ActivityDumpManager is used to manage ActivityDumps
type ActivityDumpManager struct {
	sync.RWMutex
	config                 *config.Config
	statsdClient           statsd.ClientInterface
	emptyDropped           *atomic.Uint64
	dropMaxDumpReached     *atomic.Uint64
	newEvent               func() *model.Event
	resolvers              *resolvers.EBPFResolvers
	kernelVersion          *kernel.Version
	manager                *manager.Manager
	dumpHandler            ActivityDumpHandler
	securityProfileManager SecurityProfileManager

	tracedPIDsMap          *ebpf.Map
	tracedCgroupsMap       *ebpf.Map
	cgroupWaitList         *ebpf.Map
	activityDumpsConfigMap *ebpf.Map
	ignoreFromSnapshot     map[string]bool

	dumpLimiter          *lru.Cache[cgroupModel.WorkloadSelector, *atomic.Uint64]
	workloadDenyList     []cgroupModel.WorkloadSelector
	workloadDenyListHits *atomic.Uint64

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
			if ad.GetWorkloadSelector() != nil {
				if err := adm.storage.Persist(ad); err != nil {
					seclog.Errorf("couldn't persist dump [%s]: %v", ad.GetSelectorStr(), err)
				}
			}
		} else {
			adm.emptyDropped.Inc()
		}
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

// getExpiredDumps returns the list of dumps that have timed out and remove them from the active dumps
func (adm *ActivityDumpManager) getExpiredDumps() []*ActivityDump {
	adm.Lock()
	defer adm.Unlock()

	var expiredDumps []*ActivityDump
	var newDumps []*ActivityDump
	for _, ad := range adm.activeDumps {
		if time.Now().After(ad.Metadata.End) || ad.state == Stopped {
			expiredDumps = append(expiredDumps, ad)
			delete(adm.ignoreFromSnapshot, ad.Metadata.ContainerID)
		} else {
			newDumps = append(newDumps, ad)
		}
	}
	adm.activeDumps = newDumps
	return expiredDumps
}

func (adm *ActivityDumpManager) resolveTagsPerAd(ad *ActivityDump) {
	ad.Lock()
	defer ad.Unlock()

	err := ad.resolveTags()
	if err != nil {
		seclog.Warnf("couldn't resolve activity dump tags (will try again later): %v", err)
	}

	// check if we should discard this dump based on the manager dump limiter or the deny list
	selector := ad.GetWorkloadSelector()
	if selector == nil {
		// wait for the tags
		return
	}

	shouldFinalize := false

	// check if the workload is in the deny list
	for _, entry := range adm.workloadDenyList {
		if entry.Match(*selector) {
			shouldFinalize = true
			adm.workloadDenyListHits.Inc()
			break
		}
	}

	if !shouldFinalize && !ad.countedByLimiter {
		counter, ok := adm.dumpLimiter.Get(*selector)
		if !ok {
			counter = atomic.NewUint64(0)
			adm.dumpLimiter.Add(*selector, counter)
		}

		if counter.Load() >= uint64(ad.adm.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload) {
			shouldFinalize = true
			adm.dropMaxDumpReached.Inc()
		} else {
			ad.countedByLimiter = true
			counter.Add(1)
		}
	}

	if shouldFinalize {
		ad.finalize(true)
	}
}

// resolveTags resolves activity dump container tags when they are missing
func (adm *ActivityDumpManager) resolveTags() {
	// fetch the list of dumps and release the manager as soon as possible
	adm.Lock()
	dumps := make([]*ActivityDump, len(adm.activeDumps))
	copy(dumps, adm.activeDumps)
	adm.Unlock()

	for _, ad := range dumps {
		adm.resolveTagsPerAd(ad)
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
func NewActivityDumpManager(config *config.Config, statsdClient statsd.ClientInterface, newEvent func() *model.Event, resolvers *resolvers.EBPFResolvers,
	kernelVersion *kernel.Version, manager *manager.Manager) (*ActivityDumpManager, error) {
	tracedPIDs, err := managerhelper.Map(manager, "traced_pids")
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

	limiter, err := lru.NewWithEvict(1024, func(_ cgroupModel.WorkloadSelector, _ *atomic.Uint64) {
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't create dump limiter: %w", err)
	}

	var denyList []cgroupModel.WorkloadSelector
	for _, entry := range config.RuntimeSecurity.ActivityDumpWorkloadDenyList {
		selectorTmp, err := cgroupModel.NewWorkloadSelector(entry, "*")
		if err != nil {
			return nil, fmt.Errorf("invalid workload selector in activity_dump.workload_deny_list: %w", err)
		}
		denyList = append(denyList, selectorTmp)
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
		tracedCgroupsMap:       tracedCgroupsMap,
		cgroupWaitList:         cgroupWaitList,
		activityDumpsConfigMap: activityDumpsConfigMap,
		snapshotQueue:          make(chan *ActivityDump, 100),
		ignoreFromSnapshot:     make(map[string]bool),
		dumpLimiter:            limiter,
		workloadDenyList:       denyList,
		workloadDenyListHits:   atomic.NewUint64(0),
		pathsReducer:           activity_tree.NewPathsReducer(),
	}

	adm.storage, err = NewActivityDumpStorageManager(config, statsdClient, adm, adm)
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
	for _, tag := range configUtils.GetConfiguredTags(pkgconfigsetup.Datadog(), true) {
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

	// enable the new dump to start collecting events from kernel space
	if err := newDump.enable(); err != nil {
		return fmt.Errorf("couldn't insert new dump: %w", err)
	}

	// loop through the process cache entry tree and push traced pids if necessary
	pces := adm.newProcessCacheEntrySearcher(newDump)
	adm.resolvers.ProcessResolver.Walk(pces.SearchTracedProcessCacheEntry)

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
func (adm *ActivityDumpManager) startDumpWithConfig(containerID string, containerFlags, cookie uint64, loadConfig model.ActivityDumpLoadConfig) error {
	newDump := NewActivityDump(adm, func(ad *ActivityDump) {
		ad.Metadata.ContainerID = containerID
		ad.Metadata.ContainerFlags = containerFlags
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
		return fmt.Errorf("couldn't start tracing [%s]: %v", newDump.GetSelectorStr(), err)
	}
	return nil
}

// HandleCGroupTracingEvent handles a cgroup tracing event
func (adm *ActivityDumpManager) HandleCGroupTracingEvent(event *model.CgroupTracingEvent) {
	adm.Lock()
	defer adm.Unlock()

	if len(event.ContainerContext.ContainerID) == 0 {
		seclog.Warnf("received a cgroup tracing event with an empty container ID")
		return
	}

	if err := adm.startDumpWithConfig(string(event.ContainerContext.ContainerID), uint64(event.CGroupContext.CGroupFlags), event.ConfigCookie, event.Config); err != nil {
		seclog.Warnf("%v", err)
	}
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
		if err := adm.startDumpWithConfig(string(workloads[0].ContainerID), uint64(workloads[0].CGroupFlags), utils.NewCookie(), *adm.loadController.getDefaultLoadConfig()); err != nil {
			if !errors.Is(err, unix.E2BIG) {
				seclog.Debugf("%v", err)
				break
			}
			seclog.Errorf("%v", err)
		}
	}
}

// ListActivityDumps returns the list of active activity dumps
func (adm *ActivityDumpManager) ListActivityDumps(_ *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
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

// DumpActivity handles an activity dump request
func (adm *ActivityDumpManager) DumpActivity(params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	adm.Lock()
	defer adm.Unlock()

	newDump := NewActivityDump(adm, func(ad *ActivityDump) {
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

// StopActivityDump stops an active activity dump
func (adm *ActivityDumpManager) StopActivityDump(params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	adm.Lock()
	defer adm.Unlock()

	if params.GetName() == "" && params.GetContainerID() == "" {
		errMsg := fmt.Errorf("you must specify one selector between name and containerID")
		return &api.ActivityDumpStopMessage{Error: errMsg.Error()}, errMsg
	}

	toDelete := -1
	for i, d := range adm.activeDumps {
		if (params.GetName() != "" && d.nameMatches(params.GetName())) ||
			(params.GetContainerID() != "" && d.containerIDMatches(params.GetContainerID())) {
			d.Finalize(true)
			seclog.Infof("tracing stopped for [%s]", d.GetSelectorStr())
			toDelete = i

			// persist dump if not empty
			if !d.IsEmpty() {
				if d.GetWorkloadSelector() != nil {
					if err := adm.storage.Persist(d); err != nil {
						seclog.Errorf("couldn't persist dump [%s]: %v", d.GetSelectorStr(), err)
					}
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
	} else /* if params.GetContainerID() != "" */ {
		errMsg = fmt.Errorf("the activity dump manager does not contain any ActivityDump with the following containerID: %s", params.GetContainerID())
	}
	return &api.ActivityDumpStopMessage{Error: errMsg.Error()}, errMsg
}

// HasActiveActivityDump returns true if the given event has an active dump
func (adm *ActivityDumpManager) HasActiveActivityDump(event *model.Event) bool {
	// ignore events with an error
	if event.Error != nil {
		return false
	}

	// is this event sampled for activity dumps ?
	if !event.IsActivityDumpSample() {
		return false
	}

	adm.Lock()
	defer adm.Unlock()

	for _, d := range adm.activeDumps {
		d.Lock()
		matches := d.MatchesSelector(event.ProcessCacheEntry)
		state := d.state
		d.Unlock()
		if matches && state == Running {
			return true
		}
	}

	return false
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

type processCacheEntrySearcher struct {
	adm           *ActivityDumpManager
	ad            *ActivityDump
	ancestorCache map[*model.ProcessContext]*model.ProcessCacheEntry
}

func (adm *ActivityDumpManager) newProcessCacheEntrySearcher(ad *ActivityDump) *processCacheEntrySearcher {
	return &processCacheEntrySearcher{
		adm:           adm,
		ad:            ad,
		ancestorCache: make(map[*model.ProcessContext]*model.ProcessCacheEntry),
	}
}

func (pces *processCacheEntrySearcher) getNextAncestorBinaryOrArgv0(pc *model.ProcessContext) *model.ProcessCacheEntry {
	if ancestor, ok := pces.ancestorCache[pc]; ok {
		return ancestor
	}
	newAncestor := activity_tree.GetNextAncestorBinaryOrArgv0(pc)
	pces.ancestorCache[pc] = newAncestor
	return newAncestor
}

// SearchTracedProcessCacheEntry inserts traced pids if necessary
func (pces *processCacheEntrySearcher) SearchTracedProcessCacheEntry(entry *model.ProcessCacheEntry) {
	pces.ad.Lock()
	defer pces.ad.Unlock()

	// check process lineage
	if !pces.ad.MatchesSelector(entry) {
		return
	}

	if _, err := entry.HasValidLineage(); err != nil {
		// check if the node belongs to the container
		var mn *model.ErrProcessMissingParentNode
		if !errors.As(err, &mn) {
			return
		}
	}

	// compute the list of ancestors, we need to start inserting them from the root
	ancestors := []*model.ProcessCacheEntry{entry}
	parent := pces.getNextAncestorBinaryOrArgv0(&entry.ProcessContext)
	for parent != nil && pces.ad.MatchesSelector(parent) {
		ancestors = append(ancestors, parent)
		parent = pces.getNextAncestorBinaryOrArgv0(&parent.ProcessContext)
	}
	slices.Reverse(ancestors)

	imageTag := utils.GetTagValue("image_tag", pces.ad.Tags)
	for _, parent = range ancestors {
		node, _, err := pces.ad.ActivityTree.CreateProcessNode(parent, imageTag, activity_tree.Snapshot, false, pces.adm.resolvers)
		if err != nil {
			// try to insert the other ancestors as we might find a valid root node in the lineage
			continue
		}
		if node != nil {
			// This step is important to populate the kernel space "traced_pids" map. Some traced event types use this
			// map directly (as opposed to "traced_cgroups") to determine if their events should be tagged as dump
			// samples.
			pces.ad.updateTracedPid(node.Process.Pid)
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

	if value := adm.workloadDenyListHits.Swap(0); value > 0 {
		if err := adm.statsdClient.Count(metrics.MetricActivityDumpWorkloadDenyListHits, int64(value), nil, 1.0); err != nil {
			return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpWorkloadDenyListHits, err)
		}
	}

	adm.storage.SendTelemetry()

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
		// restart a new dump for the same workload
		newDump := adm.loadController.NextPartialDump(ad)

		// stop the dump but do not release the cgroup
		ad.Finalize(false)
		seclog.Infof("tracing paused for [%s]", ad.GetSelectorStr())

		// persist dump if not empty
		if !ad.IsEmpty() {
			if ad.GetWorkloadSelector() != nil {
				if err := adm.storage.Persist(ad); err != nil {
					seclog.Errorf("couldn't persist dump [%s]: %v", ad.GetSelectorStr(), err)
				}
			}
		} else {
			adm.emptyDropped.Inc()
		}

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
	defer adm.Unlock()

	for _, ad := range adm.activeDumps {
		ad.Lock()
		if adSelector := ad.GetWorkloadSelector(); adSelector != nil && adSelector.Match(selector) {
			ad.finalize(true)
			adm.dropMaxDumpReached.Inc()
		}
		ad.Unlock()
	}
}
