// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	ebpfmanager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/hostnameutils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// ActivityDumpSource defines the source of activity dumps
	ActivityDumpSource         = "runtime-security-agent"
	absoluteMinimumDumpTimeout = 10 * time.Second
)

var (
	// TracedEventTypesReductionOrder is the order by which event types are reduced
	TracedEventTypesReductionOrder = []model.EventType{model.BindEventType, model.IMDSEventType, model.DNSEventType, model.SyscallsEventType, model.FileOpenEventType}
)

// Manager is the manager for activity dumps and security profiles
type Manager struct {
	m sync.Mutex

	config        *config.Config
	statsdClient  statsd.ClientInterface
	resolvers     *resolvers.EBPFResolvers
	kernelVersion *kernel.Version
	newEvent      func() *model.Event
	pathsReducer  *activity_tree.PathsReducer

	// fields from ActivityDumpManager
	activityDumpLoadConfig map[containerutils.CGroupManager]*model.ActivityDumpLoadConfig

	// ebpf maps
	tracedPIDsMap              *ebpf.Map
	tracedCgroupsMap           *ebpf.Map
	cgroupWaitList             *ebpf.Map
	activityDumpsConfigMap     *ebpf.Map
	activityDumpConfigDefaults *ebpf.Map

	ignoreFromSnapshot   map[model.PathKey]bool
	dumpLimiter          *lru.Cache[cgroupModel.WorkloadSelector, *atomic.Uint64]
	workloadDenyList     []cgroupModel.WorkloadSelector
	workloadDenyListHits *atomic.Uint64

	// storage
	localStorage              *storage.Directory
	remoteStorage             *storage.ActivityDumpRemoteStorageForwarder
	configuredStorageRequests map[config.StorageFormat][]config.StorageRequest

	activeDumps         []*dump.ActivityDump
	snapshotQueue       chan *dump.ActivityDump
	contextTags         []string
	hostname            string
	lastStoppedDumpTime time.Time

	// ActivityDumpLoadController
	minDumpTimeout time.Duration

	// stats
	emptyDropped       *atomic.Uint64
	dropMaxDumpReached *atomic.Uint64

	// fields from SecurityProfileManager

	secProfEventTypes []model.EventType

	// ebpf maps
	securityProfileMap         *ebpf.Map
	securityProfileSyscallsMap *ebpf.Map

	profilesLock        sync.Mutex
	profiles            map[cgroupModel.WorkloadSelector]*profile.Profile
	evictedVersionsLock sync.Mutex
	evictedVersions     []cgroupModel.WorkloadSelector

	pendingCacheLock sync.Mutex
	pendingCache     *simplelru.LRU[cgroupModel.WorkloadSelector, *profile.Profile]
	cacheHit         *atomic.Uint64
	cacheMiss        *atomic.Uint64

	// event filtering
	eventFiltering map[eventFilteringEntry]*atomic.Uint64

	// chan used to move an ActivityDump profile to a SecurityProfile profile
	newProfiles chan *profile.Profile
}

// NewManager returns a new instance of the security profile manager
func NewManager(cfg *config.Config, statsdClient statsd.ClientInterface, ebpf *ebpfmanager.Manager, resolvers *resolvers.EBPFResolvers, kernelVersion *kernel.Version, newEvent func() *model.Event, dumpHandler storage.ActivityDumpHandler) (*Manager, error) {
	tracedPIDs, err := managerhelper.Map(ebpf, "traced_pids")
	if err != nil {
		return nil, err
	}

	tracedCgroupsMap, err := managerhelper.Map(ebpf, "traced_cgroups")
	if err != nil {
		return nil, err
	}

	activityDumpsConfigMap, err := managerhelper.Map(ebpf, "activity_dumps_config")
	if err != nil {
		return nil, err
	}

	cgroupWaitList, err := managerhelper.Map(ebpf, "cgroup_wait_list")
	if err != nil {
		return nil, err
	}

	activityDumpConfigDefaultsMap, err := managerhelper.Map(ebpf, "activity_dump_config_defaults")
	if err != nil {
		return nil, err
	}

	securityProfileMap, err := managerhelper.Map(ebpf, "security_profiles")
	if err != nil {
		return nil, err
	}

	securityProfileSyscallsMap, err := managerhelper.Map(ebpf, "secprofs_syscalls")
	if err != nil {
		return nil, err
	}

	minDumpTimeout := cfg.RuntimeSecurity.ActivityDumpLoadControlMinDumpTimeout
	if minDumpTimeout < absoluteMinimumDumpTimeout {
		minDumpTimeout = absoluteMinimumDumpTimeout
	}

	dumpLimiter, err := lru.New[cgroupModel.WorkloadSelector, *atomic.Uint64](1024)
	if err != nil {
		return nil, fmt.Errorf("couldn't create dump limiter: %w", err)
	}

	var workloadDenyList []cgroupModel.WorkloadSelector
	for _, entry := range cfg.RuntimeSecurity.ActivityDumpWorkloadDenyList {
		selectorTmp, err := cgroupModel.NewWorkloadSelector(entry, "*")
		if err != nil {
			return nil, fmt.Errorf("invalid workload selector in activity_dump.workload_deny_list: %w", err)
		}
		workloadDenyList = append(workloadDenyList, selectorTmp)
	}

	localStorage, err := storage.NewDirectory(cfg.RuntimeSecurity.ActivityDumpLocalStorageDirectory, cfg.RuntimeSecurity.ActivityDumpLocalStorageMaxDumpsCount)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate the local storage: %w", err)
	}

	remoteStorage, err := storage.NewActivityDumpRemoteStorageForwarder(dumpHandler)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate the remote storage forwarder: %w", err)
	}

	var configuredStorageRequests []config.StorageRequest
	for _, format := range cfg.RuntimeSecurity.ActivityDumpLocalStorageFormats {
		configuredStorageRequests = append(configuredStorageRequests, config.NewStorageRequest(
			config.LocalStorage,
			format,
			cfg.RuntimeSecurity.ActivityDumpLocalStorageCompression,
			cfg.RuntimeSecurity.ActivityDumpLocalStorageDirectory,
		))
	}

	// add remote storage requests
	configuredStorageRequests = append(configuredStorageRequests, config.NewStorageRequest(
		config.RemoteStorage,
		config.Protobuf,
		true, // force remote compression
		"",
	))

	hostname, err := hostnameutils.GetHostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	contextTags := []string{"host:" + hostname}
	// merge tags from config
	for _, tag := range configUtils.GetConfiguredTags(pkgconfigsetup.Datadog(), true) {
		if strings.HasPrefix(tag, "host") {
			continue
		}
		contextTags = append(contextTags, tag)
	}
	// add source tag
	if len(utils.GetTagValue("source", contextTags)) == 0 {
		contextTags = append(contextTags, fmt.Sprintf("source:%s", ActivityDumpSource))
	}

	profileCache, err := simplelru.NewLRU[cgroupModel.WorkloadSelector, *profile.Profile](cfg.RuntimeSecurity.SecurityProfileCacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create security profile cache: %w", err)
	}

	var secProfEventTypes []model.EventType
	if cfg.RuntimeSecurity.SecurityProfileAutoSuppressionEnabled {
		secProfEventTypes = append(secProfEventTypes, cfg.RuntimeSecurity.SecurityProfileAutoSuppressionEventTypes...)
	}
	if cfg.RuntimeSecurity.AnomalyDetectionEnabled {
		secProfEventTypes = append(secProfEventTypes, cfg.RuntimeSecurity.AnomalyDetectionEventTypes...)
	}
	// merge and remove duplicated event types
	slices.Sort(secProfEventTypes)
	secProfEventTypes = slices.Clip(slices.Compact(secProfEventTypes))

	m := &Manager{
		config:        cfg,
		statsdClient:  statsdClient,
		resolvers:     resolvers,
		kernelVersion: kernelVersion,
		newEvent:      newEvent,
		pathsReducer:  activity_tree.NewPathsReducer(),

		tracedPIDsMap:              tracedPIDs,
		tracedCgroupsMap:           tracedCgroupsMap,
		cgroupWaitList:             cgroupWaitList,
		activityDumpsConfigMap:     activityDumpsConfigMap,
		activityDumpConfigDefaults: activityDumpConfigDefaultsMap,

		ignoreFromSnapshot:   make(map[model.PathKey]bool),
		dumpLimiter:          dumpLimiter,
		workloadDenyList:     workloadDenyList,
		workloadDenyListHits: atomic.NewUint64(0),

		snapshotQueue:             make(chan *dump.ActivityDump, 100),
		localStorage:              localStorage,
		remoteStorage:             remoteStorage,
		configuredStorageRequests: perFormatStorageRequests(configuredStorageRequests),

		contextTags: contextTags,
		hostname:    hostname,

		minDumpTimeout: minDumpTimeout,

		emptyDropped:       atomic.NewUint64(0),
		dropMaxDumpReached: atomic.NewUint64(0),

		secProfEventTypes: secProfEventTypes,

		securityProfileMap:         securityProfileMap,
		securityProfileSyscallsMap: securityProfileSyscallsMap,

		profiles: make(map[cgroupModel.WorkloadSelector]*profile.Profile),

		pendingCache: profileCache,
		cacheHit:     atomic.NewUint64(0),
		cacheMiss:    atomic.NewUint64(0),

		eventFiltering: make(map[eventFilteringEntry]*atomic.Uint64),

		newProfiles: make(chan *profile.Profile, 100),
	}

	m.initMetricsMap()

	defaultLoadConfigs, err := m.getDefaultLoadConfigs()
	if err != nil {
		return nil, fmt.Errorf("couldn't get default load configs: %w", err)
	}

	// push default load config values
	for cgroupManager, defaultConfig := range defaultLoadConfigs {
		if err := m.activityDumpConfigDefaults.Put(uint32(cgroupManager), defaultConfig); err != nil {
			return nil, fmt.Errorf("couldn't update default activity dump load config for manager %s: %w", cgroupManager.String(), err)
		}
	}

	return m, nil
}

func (m *Manager) initMetricsMap() {
	for i := model.EventType(0); i < model.MaxKernelEventType; i++ {
		for _, state := range model.AllEventFilteringProfileState {
			for _, result := range allEventFilteringResults {
				m.eventFiltering[eventFilteringEntry{
					eventType: i,
					state:     state,
					result:    result,
				}] = atomic.NewUint64(0)
			}
		}
	}
}

// ProcessEvent processes a new event and insert it in an activity dump if applicable
func (m *Manager) ProcessEvent(event *model.Event) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return
	}

	if event.Error != nil {
		return
	}

	if !event.IsActivityDumpSample() {
		return
	}

	m.m.Lock()
	defer m.m.Unlock()

	for _, ad := range m.activeDumps {
		inserted, size, _ := ad.Insert(event, m.resolvers)
		if inserted && size >= int64(m.config.RuntimeSecurity.ActivityDumpMaxDumpSize()) {
			if err := m.pauseKernelEventCollection(ad); err != nil {
				seclog.Warnf("couldn't pause max-sized activity dump: %v", err)
			}
		}
	}
}

// HasActiveActivityDump returns true if the given event has an active dump
func (m *Manager) HasActiveActivityDump(event *model.Event) bool {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return false
	}

	// ignore events with an error
	if event.Error != nil {
		return false
	}

	// is this event sampled for activity dumps ?
	if !event.IsActivityDumpSample() {
		return false
	}

	m.m.Lock()
	defer m.m.Unlock()

	for _, d := range m.activeDumps {
		if d.GetState() == dump.Running && d.MatchesSelector(event.ProcessCacheEntry) {
			return true
		}
	}

	return false
}

// Start runs the manager
func (m *Manager) Start(ctx context.Context) {
	var adCleanupTickerChan <-chan time.Time
	var adTagsTickerChan <-chan time.Time
	var adLoadControlTickerChan <-chan time.Time
	var silentWorkloadsTickerChan <-chan time.Time

	if m.config.RuntimeSecurity.ActivityDumpEnabled {
		adCleanupTicker := time.NewTicker(m.config.RuntimeSecurity.ActivityDumpCleanupPeriod)
		defer adCleanupTicker.Stop()
		adCleanupTickerChan = adCleanupTicker.C

		adTagsTickerChanTimer := time.NewTicker(m.config.RuntimeSecurity.ActivityDumpTagsResolutionPeriod)
		defer adTagsTickerChanTimer.Stop()
		adTagsTickerChan = adTagsTickerChanTimer.C

		adLoadControlTicker := time.NewTicker(m.config.RuntimeSecurity.ActivityDumpLoadControlPeriod)
		defer adLoadControlTicker.Stop()
		adLoadControlTickerChan = adLoadControlTicker.C
	} else {
		adCleanupTickerChan = make(chan time.Time)
		adTagsTickerChan = make(chan time.Time)
		adLoadControlTickerChan = make(chan time.Time)
	}

	if m.config.RuntimeSecurity.ActivityDumpEnabled && m.config.RuntimeSecurity.SecurityProfileEnabled {
		silentWorkloadsTicker := time.NewTicker(m.config.RuntimeSecurity.ActivityDumpSilentWorkloadsTicker)
		defer silentWorkloadsTicker.Stop()
		silentWorkloadsTickerChan = silentWorkloadsTicker.C
	} else {
		silentWorkloadsTickerChan = make(chan time.Time)
	}

	if m.config.RuntimeSecurity.SecurityProfileEnabled {
		_ = m.resolvers.TagsResolver.RegisterListener(tags.WorkloadSelectorResolved, m.onWorkloadSelectorResolvedEvent)
		_ = m.resolvers.TagsResolver.RegisterListener(tags.WorkloadSelectorDeleted, m.onWorkloadDeletedEvent)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	seclog.Infof("security profile manager started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-adCleanupTickerChan:
			m.cleanup()
		case <-adTagsTickerChan:
			m.resolveTagsAll()
		case <-adLoadControlTickerChan:
			m.triggerLoadController()
		case ad := <-m.snapshotQueue:
			if err := m.snapshot(ad); err != nil {
				seclog.Errorf("couldn't snapshot [%s]: %v", ad.Profile.Metadata.ContainerID, err)
			}
		case <-silentWorkloadsTickerChan:
			m.handleSilentWorkloads()
		case newProfile := <-m.newProfiles:
			m.onNewProfile(newProfile)
		}
	}
}

// getExpiredDumps returns the list of dumps that have timed out and remove them from the active dumps
func (m *Manager) getExpiredDumps() []*dump.ActivityDump {
	m.m.Lock()
	defer m.m.Unlock()

	var expiredDumps []*dump.ActivityDump
	var newDumps []*dump.ActivityDump
	for _, ad := range m.activeDumps {
		if time.Now().After(ad.Profile.Metadata.End) || ad.GetState() == dump.Stopped {
			expiredDumps = append(expiredDumps, ad)
			delete(m.ignoreFromSnapshot, ad.Profile.Metadata.CGroupContext.CGroupFile)
		} else {
			newDumps = append(newDumps, ad)
		}
	}

	m.activeDumps = newDumps
	return expiredDumps
}

// cleanup
func (m *Manager) cleanup() {
	// fetch expired dumps
	dumps := m.getExpiredDumps()

	for _, ad := range dumps {
		m.finalizeKernelEventCollection(ad, true)
		seclog.Infof("tracing stopped for [%s]", ad.GetSelectorStr())

		// persist dump if not empty
		if !ad.Profile.IsEmpty() && ad.Profile.GetWorkloadSelector() != nil {
			if err := m.persist(ad.Profile, m.configuredStorageRequests); err != nil {
				seclog.Errorf("couldn't persist dump [%s]: %v", ad.GetSelectorStr(), err)
			} else if m.config.RuntimeSecurity.SecurityProfileEnabled { // drop the profile if we don't care about using it as a security profile
				select {
				case m.newProfiles <- ad.Profile:
				default:
					// drop the profile and log error if the channel is full
					seclog.Warnf("couldn't send new profile to the manager: channel is full")
				}
			}
		} else {
			m.emptyDropped.Inc()
		}
	}

	// cleanup cgroup_wait_list map
	iterator := m.cgroupWaitList.Iterate()
	cgroupFile := make([]byte, model.PathKeySize)
	var timestamp uint64

	for iterator.Next(&cgroupFile, &timestamp) {
		if time.Now().After(m.resolvers.TimeResolver.ResolveMonotonicTimestamp(timestamp)) {
			if err := m.cgroupWaitList.Delete(&cgroupFile); err != nil {
				seclog.Errorf("couldn't delete cgroup_wait_list entry for (%v): %v", cgroupFile, err)
			}
		}
	}
}

// Activity Dump
func (m *Manager) newActivityDumpLoadConfig(evt []model.EventType, timeout time.Duration, waitListTimeout time.Duration, rate uint16, start time.Time, flags containerutils.CGroupFlags) *model.ActivityDumpLoadConfig {
	lc := &model.ActivityDumpLoadConfig{
		TracedEventTypes: evt,
		Timeout:          timeout,
		Rate:             uint16(rate),
		CGroupFlags:      flags,
	}
	if m.resolvers != nil {
		lc.StartTimestampRaw = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(start))
		lc.EndTimestampRaw = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(start.Add(timeout)))
		lc.WaitListTimestampRaw = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(start.Add(waitListTimeout)))
	}
	return lc
}

func (m *Manager) defaultActivityDumpLoadConfig(now time.Time, flags containerutils.CGroupFlags) *model.ActivityDumpLoadConfig {
	return m.newActivityDumpLoadConfig(
		m.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		m.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout,
		m.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout,
		m.config.RuntimeSecurity.ActivityDumpRateLimiter,
		now,
		flags,
	)
}

func (m *Manager) getDefaultLoadConfigs() (map[containerutils.CGroupManager]*model.ActivityDumpLoadConfig, error) {
	if m.activityDumpLoadConfig != nil {
		return m.activityDumpLoadConfig, nil
	}

	defaults := m.defaultActivityDumpLoadConfig(time.Now(), containerutils.CGroupFlags(0)) // cgroup flags will be set per cgroup manager

	allDefaultConfigs := map[string]containerutils.CGroupManager{
		containerutils.CGroupManagerDocker.String():  containerutils.CGroupManagerDocker,
		containerutils.CGroupManagerPodman.String():  containerutils.CGroupManagerPodman,
		containerutils.CGroupManagerCRI.String():     containerutils.CGroupManagerCRI,
		containerutils.CGroupManagerCRIO.String():    containerutils.CGroupManagerCRIO,
		containerutils.CGroupManagerSystemd.String(): containerutils.CGroupManagerSystemd,
	}
	defaultConfigs := make(map[containerutils.CGroupManager]*model.ActivityDumpLoadConfig)
	for _, cgroupManager := range m.config.RuntimeSecurity.ActivityDumpCgroupsManagers {
		cgroupManager, found := allDefaultConfigs[cgroupManager]
		if !found {
			return nil, fmt.Errorf("unsupported cgroup manager '%s'", cgroupManager)
		}
		cgroupManagerLoadConfig := *defaults
		cgroupManagerLoadConfig.CGroupFlags = containerutils.CGroupFlags(cgroupManager)
		defaultConfigs[cgroupManager] = &cgroupManagerLoadConfig
	}

	m.activityDumpLoadConfig = defaultConfigs
	return defaultConfigs, nil
}

// insertActivityDump inserts an activity dump in the list of activity dumps handled by the manager
func (m *Manager) insertActivityDump(newDump *dump.ActivityDump) error {
	// sanity checks
	if len(newDump.Profile.Metadata.ContainerID) > 0 {
		// check if the provided container ID is new
		for _, ad := range m.activeDumps {
			if ad.Profile.Metadata.ContainerID == newDump.Profile.Metadata.ContainerID {
				// an activity dump is already active for this container ID, ignore
				return fmt.Errorf("dump for container %s already running", ad.Profile.Metadata.ContainerID)
			}
		}
	}

	if len(newDump.Profile.Metadata.CGroupContext.CGroupID) > 0 {
		// check if the provided cgroup ID is new
		for _, ad := range m.activeDumps {
			if ad.Profile.Metadata.CGroupContext.CGroupID == newDump.Profile.Metadata.CGroupContext.CGroupID {
				// an activity dump is already active for this cgroup ID, ignore
				return fmt.Errorf("dump for cgroup %s already running", ad.Profile.Metadata.CGroupContext.CGroupID)
			}
		}
	}

	// loop through the process cache entry tree and push traced pids if necessary
	pces := m.newProcessCacheEntrySearcher(newDump)
	m.resolvers.ProcessResolver.Walk(func(entry *model.ProcessCacheEntry) {
		if !pces.ad.MatchesSelector(entry) {
			return
		}
		pces.ad.Profile.Metadata.CGroupContext = entry.CGroup
		pces.searchTracedProcessCacheEntry(entry)
	})

	// enable the new dump to start collecting events from kernel space
	if err := m.enableKernelEventCollection(newDump); err != nil {
		return fmt.Errorf("couldn't insert new dump: %w", err)
	}

	// Delay the activity dump snapshot to reduce the overhead on the main goroutine
	select {
	case m.snapshotQueue <- newDump:
	default:
	}

	// set the AD state now so that we can start inserting new events
	newDump.SetState(dump.Running)

	// append activity dump to the list of active dumps
	m.activeDumps = append(m.activeDumps, newDump)

	seclog.Infof("tracing started for [%s]", newDump.GetSelectorStr())
	return nil
}

const systemdSystemDir = "/usr/lib/systemd/system"

// resolveTags thread unsafe version ot ResolveTags
func (m *Manager) resolveTags(ad *dump.ActivityDump) error {
	selector := ad.Profile.GetWorkloadSelector()
	if selector != nil {
		return nil
	}

	if len(ad.Profile.Metadata.ContainerID) > 0 {

		tags, err := m.resolvers.TagsResolver.ResolveWithErr(containerutils.ContainerID(ad.Profile.Metadata.ContainerID))
		if err != nil {
			return fmt.Errorf("failed to resolve %s: %w", ad.Profile.Metadata.ContainerID, err)
		}

		ad.Profile.AddTags(tags)
	} else if len(ad.Profile.Metadata.CGroupContext.CGroupID) > 0 {
		systemdService := filepath.Base(string(ad.Profile.Metadata.CGroupContext.CGroupID))
		serviceVersion := ""
		servicePath := filepath.Join(systemdSystemDir, systemdService)

		if m.resolvers.SBOMResolver != nil {
			if pkg := m.resolvers.SBOMResolver.ResolvePackage("", &model.FileEvent{PathnameStr: servicePath}); pkg != nil {
				serviceVersion = pkg.Version
			}
		}

		ad.Profile.AddTags([]string{
			"service:" + systemdService,
			"version:" + serviceVersion,
		})
	}

	ad.Profile.AddTags([]string{
		"cgroup_manager:" + containerutils.CGroupManager(ad.Profile.Metadata.CGroupContext.CGroupFlags&containerutils.CGroupManagerMask).String(),
	})

	return nil
}

// resolveTags resolves activity dump container tags when they are missing
func (m *Manager) resolveTagsAll() {
	m.m.Lock()
	defer m.m.Unlock()

	for _, ad := range m.activeDumps {
		m.resolveTagsPerAd(ad)
	}
}

// resolveTagsPerAd resolves the tags for a single activity dump
func (m *Manager) resolveTagsPerAd(ad *dump.ActivityDump) {
	err := m.resolveTags(ad)
	if err != nil {
		seclog.Warnf("couldn't resolve activity dump tags (will try again later): %v", err)
	}

	// check if we should discard this dump based on the manager dump limiter or the deny list
	selector := ad.Profile.GetWorkloadSelector()
	if selector == nil {
		// wait for the tags
		return
	}

	shouldFinalize := false
	for _, entry := range m.workloadDenyList {
		if entry.Match(*selector) {
			shouldFinalize = true
			m.workloadDenyListHits.Inc()
			break
		}
	}

	if !shouldFinalize && !ad.IsCountedByLimiter() {
		counter, ok := m.dumpLimiter.Get(*selector)
		if !ok {
			counter = atomic.NewUint64(0)
			m.dumpLimiter.Add(*selector, counter)
		}

		if counter.Load() >= uint64(m.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload) {
			shouldFinalize = true
			m.dropMaxDumpReached.Inc()
		} else {
			ad.SetCountedByLimiter(true)
			counter.Add(1)
		}
	}

	if shouldFinalize {
		m.finalizeKernelEventCollection(ad, true)
	}
}

func (m *Manager) snapshot(ad *dump.ActivityDump) error {
	ad.Profile.Snapshot(m.newEvent)

	// try to resolve the tags now
	_ = m.resolveTags(ad)
	return nil
}

func (m *Manager) enableKernelEventCollection(ad *dump.ActivityDump) error {
	// insert load config now (it might already exist when starting a new partial dump, update it in that case)
	if err := m.activityDumpsConfigMap.Update(ad.Cookie, ad.LoadConfig.Load(), ebpf.UpdateAny); err != nil {
		if !errors.Is(err, ebpf.ErrKeyExist) {
			return fmt.Errorf("couldn't push activity dump load config: %w", err)
		}
	}

	if !ad.Profile.Metadata.CGroupContext.CGroupFile.IsNull() {
		// insert container ID in traced_cgroups map (it might already exist, do not update in that case)
		if err := m.tracedCgroupsMap.Update(ad.Profile.Metadata.CGroupContext.CGroupFile, ad.Cookie, ebpf.UpdateNoExist); err != nil {
			if !errors.Is(err, ebpf.ErrKeyExist) {
				// delete activity dump load config
				_ = m.activityDumpsConfigMap.Delete(ad.Cookie)
				return fmt.Errorf("couldn't push activity dump cgroup ID %s: %w", ad.GetSelectorStr(), err)
			}
		}
	}

	return nil
}

// pause (thread unsafe) assuming the current dump is running, "pause" sets the kernel space filters of the dump so that
// events are ignored in kernel space, and not sent to user space.
func (m *Manager) pauseKernelEventCollection(ad *dump.ActivityDump) error {
	if ad.GetState() <= dump.Paused {
		// nothing to do
		return nil
	}
	ad.SetState(dump.Paused)

	newLoadConfig := *ad.LoadConfig.Load()
	newLoadConfig.Paused = 1
	ad.LoadConfig.Store(&newLoadConfig)
	if err := m.activityDumpsConfigMap.Put(ad.Cookie, newLoadConfig); err != nil {
		return fmt.Errorf("failed to pause activity dump [%s]: %w", ad.Profile.Metadata.ContainerID, err)
	}

	return nil
}

// disable (thread unsafe) assuming the current dump is running, "disable" removes kernel space filters so that events are no longer sent
// from kernel space
func (m *Manager) disableKernelEventCollection(ad *dump.ActivityDump) error {
	if ad.GetState() <= dump.Disabled {
		// nothing to do
		return nil
	}
	ad.SetState(dump.Disabled)

	// remove activity dump
	if err := m.activityDumpsConfigMap.Delete(ad.Cookie); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		return fmt.Errorf("couldn't delete activity dump load config for dump [%s]: %w", ad.GetSelectorStr(), err)
	}

	if !ad.Profile.Metadata.CGroupContext.CGroupFile.IsNull() {
		err := m.tracedCgroupsMap.Delete(ad.Profile.Metadata.CGroupContext.CGroupFile)
		if err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return fmt.Errorf("couldn't delete activity dump filter cgroup %s: %v", ad.GetSelectorStr(), err)
		}
	}

	return nil
}

// finalize (thread unsafe) finalizes an active dump: envs and args are scrubbed, tags, service and container ID are set. If a cgroup
// spot can be released, the dump will be fully stopped.
func (m *Manager) finalizeKernelEventCollection(ad *dump.ActivityDump, releaseTracedCgroupSpot bool) {
	if ad.GetState() == dump.Stopped {
		return
	}

	now := time.Now()
	ad.Profile.Metadata.End = now
	m.lastStoppedDumpTime = now

	if releaseTracedCgroupSpot {
		if err := m.disableKernelEventCollection(ad); err != nil {
			seclog.Errorf("couldn't disable activity dump: %v", err)
		}

		ad.SetState(dump.Stopped)
	}

	// add additional tags
	ad.Profile.AddTags(m.contextTags)

	// look for the service tag and set the service of the dump
	ad.Profile.Header.Service = ad.Profile.GetTagValue("service")

	// add the container ID in a tag
	if len(ad.Profile.Metadata.ContainerID) > 0 {
		// make sure we are not adding the same tag twice
		newTag := fmt.Sprintf("container_id:%s", ad.Profile.Metadata.ContainerID)
		if !ad.Profile.HasTag(newTag) {
			ad.Profile.AddTags([]string{newTag})
		} else {
			seclog.Errorf("container_id tag already present in tags (is finalize called multiple times?): %s", newTag)
		}
	}

	// add VersionContext
	if selector := ad.Profile.GetWorkloadSelector(); selector != nil && selector.IsReady() {
		nowNano := uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(now))
		tags := ad.Profile.GetTags()
		vCtx := &profile.VersionContext{
			FirstSeenNano:  nowNano,
			LastSeenNano:   nowNano,
			EventTypeState: make(map[model.EventType]*profile.EventTypeState),
			Syscalls:       ad.Profile.ComputeSyscallsList(),
			Tags:           make([]string, len(tags)),
		}
		copy(vCtx.Tags, tags)

		ad.Profile.AddVersionContext(selector.Tag, vCtx)
	}

	// scrub processes and retain args envs now
	ad.Profile.ScrubProcessArgsEnvs(m.resolvers.ProcessResolver)
}

// getOverweightDumps returns the list of dumps that crossed the config.ActivityDumpMaxDumpSize threshold
func (m *Manager) getOverweightDumps() []*dump.ActivityDump {
	var dumps []*dump.ActivityDump
	var toDelete []int
	for i, ad := range m.activeDumps {
		dumpSize := ad.Profile.ComputeInMemorySize()

		// send dump size in memory metric
		if err := m.statsdClient.Gauge(metrics.MetricActivityDumpActiveDumpSizeInMemory, float64(dumpSize), []string{fmt.Sprintf("dump_index:%d", i)}, 1); err != nil {
			seclog.Errorf("couldn't send %s metric: %v", metrics.MetricActivityDumpActiveDumpSizeInMemory, err)
		}

		if dumpSize >= int64(m.config.RuntimeSecurity.ActivityDumpMaxDumpSize()) {
			toDelete = append([]int{i}, toDelete...)
			dumps = append(dumps, ad)
			m.ignoreFromSnapshot[ad.Profile.Metadata.CGroupContext.CGroupFile] = true
		}
	}
	for _, i := range toDelete {
		m.activeDumps = append(m.activeDumps[:i], m.activeDumps[i+1:]...)
	}
	return dumps
}

// FakeDumpOverweight fakes a dump stats to force triggering the load controller. For unitary tests purpose only.
func (m *Manager) FakeDumpOverweight(name string) {
	m.m.Lock()
	defer m.m.Unlock()
	for _, p := range m.activeDumps {
		if p.Profile.Metadata.Name == name {
			p.Profile.FakeOverweight()
		}
	}
}

// stopDumpsWithSelector stops the active dumps for the given selector and prevent a workload with the provided selector from ever being dumped again
func (m *Manager) stopDumpsWithSelector(selector cgroupModel.WorkloadSelector) {
	counter, ok := m.dumpLimiter.Get(selector)
	if !ok {
		counter = atomic.NewUint64(uint64(m.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload))
		m.dumpLimiter.Add(selector, counter)
	} else {
		if counter.Load() < uint64(m.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload) {
			seclog.Infof("activity dumps will no longer be generated for %s", selector.String())
			counter.Store(uint64(m.config.RuntimeSecurity.ActivityDumpMaxDumpCountPerWorkload))
		}
	}

	m.m.Lock()
	defer m.m.Unlock()

	for _, ad := range m.activeDumps {
		if adSelector := ad.Profile.GetWorkloadSelector(); adSelector != nil && adSelector.Match(selector) {
			m.finalizeKernelEventCollection(ad, true)
			m.dropMaxDumpReached.Inc()
		}
	}
}

// loadController

func (m *Manager) sendLoadControllerTriggeredMetric(tags []string) error {
	if err := m.statsdClient.Count(metrics.MetricActivityDumpLoadControllerTriggered, 1, tags, 1.0); err != nil {
		return fmt.Errorf("couldn't send %s metric: %v", metrics.MetricActivityDumpLoadControllerTriggered, err)
	}
	return nil
}

func (m *Manager) nextPartialDump(prev *dump.ActivityDump) *dump.ActivityDump {
	previousLoadConfig := prev.LoadConfig.Load()
	timeToThreshold := time.Since(prev.Profile.Metadata.Start)

	newRate := previousLoadConfig.Rate
	if timeToThreshold < m.minDumpTimeout {
		newRate = previousLoadConfig.Rate * 3 / 4 // reduce by 25%
		if err := m.sendLoadControllerTriggeredMetric([]string{"reduction:rate"}); err != nil {
			seclog.Errorf("%v", err)
		}
	}

	newTimeout := previousLoadConfig.Timeout
	if timeToThreshold < m.minDumpTimeout/2 && previousLoadConfig.Timeout > m.minDumpTimeout {
		newTimeout = previousLoadConfig.Timeout * 3 / 4 // reduce by 25%
		if newTimeout < m.minDumpTimeout {
			newTimeout = m.minDumpTimeout
		}
		if err := m.sendLoadControllerTriggeredMetric([]string{"reduction:dump_timeout"}); err != nil {
			seclog.Errorf("%v", err)
		}
	}

	newEvents := make([]model.EventType, len(previousLoadConfig.TracedEventTypes))
	copy(newEvents, previousLoadConfig.TracedEventTypes)
	if timeToThreshold < m.minDumpTimeout/4 {
		var evtToRemove model.EventType
		newEvents = newEvents[:0]
	reductionOrder:
		for _, evt := range TracedEventTypesReductionOrder {
			for _, tracedEvt := range previousLoadConfig.TracedEventTypes {
				if evt == tracedEvt {
					evtToRemove = evt
					break reductionOrder
				}
			}
		}
		for _, evt := range previousLoadConfig.TracedEventTypes {
			if evt != evtToRemove {
				newEvents = append(newEvents, evt)
			}
		}

		if evtToRemove != model.UnknownEventType {
			if err := m.sendLoadControllerTriggeredMetric([]string{"reduction:traced_event_types", "event_type:" + evtToRemove.String()}); err != nil {
				seclog.Errorf("%v", err)
			}
		}
	}

	now := time.Now()
	newLoadConfig := m.newActivityDumpLoadConfig(newEvents, newTimeout, m.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout, newRate, now, previousLoadConfig.CGroupFlags)
	newDump := dump.NewActivityDump(m.pathsReducer, prev.Profile.Metadata.DifferentiateArgs, 0, m.config.RuntimeSecurity.ActivityDumpTracedEventTypes, m.updateTracedPid, newLoadConfig, func(ad *dump.ActivityDump) {
		ad.Profile.Header = prev.Profile.Header
		ad.Profile.Metadata = prev.Profile.Metadata
		ad.Profile.Metadata.Name = fmt.Sprintf("activity-dump-%s", utils.RandString(10))
		ad.Profile.Metadata.Start = now
		ad.Profile.Metadata.End = now.Add(newTimeout)
		ad.Profile.AddTags(prev.Profile.GetTags())
	})

	newDump.Cookie = prev.Cookie

	return newDump
}

func (m *Manager) triggerLoadController() {
	m.m.Lock()
	defer m.m.Unlock()

	// handle overweight dumps
	for _, ad := range m.getOverweightDumps() {
		// restart a new dump for the same workload
		newDump := m.nextPartialDump(ad)

		// stop the dump but do not release the cgroup
		m.finalizeKernelEventCollection(ad, false)
		seclog.Infof("tracing paused for [%s]", ad.GetSelectorStr())

		// persist dump if not empty
		if !ad.Profile.IsEmpty() && ad.Profile.GetWorkloadSelector() != nil {
			if err := m.persist(ad.Profile, m.configuredStorageRequests); err != nil {
				seclog.Errorf("couldn't persist dump [%s]: %v", ad.GetSelectorStr(), err)
			} else if m.config.RuntimeSecurity.SecurityProfileEnabled { // drop the profile if we don't care about using it as a security profile
				select {
				case m.newProfiles <- ad.Profile:
				default:
					// drop the profile and log error if the channel is full
					seclog.Warnf("couldn't send new profile to the manager: channel is full")
				}
			}
		} else {
			m.emptyDropped.Inc()
		}

		if err := m.insertActivityDump(newDump); err != nil {
			seclog.Errorf("couldn't resume tracing [%s]: %v", newDump.GetSelectorStr(), err)
		}

		// remove container ID from the map of ignored container IDs for the snapshot
		delete(m.ignoreFromSnapshot, ad.Profile.Metadata.CGroupContext.CGroupFile)
	}
}

// Activity dump creations

// handleSilentWorkloads checks if we should start tracing one of the workloads from a profile without an activity tree of the Security Profile manager
func (m *Manager) handleSilentWorkloads() {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return
	}

	m.m.Lock()
	defer m.m.Unlock()

	// check if it's a good time to look for a silent workload, to do so, check if the last stopped dump was stopped more
	// than the configured amount of time ago
	if time.Since(m.lastStoppedDumpTime) < m.config.RuntimeSecurity.ActivityDumpSilentWorkloadsDelay {
		return
	}

	// if we're already at capacity leave now - this prevents an unnecessary lock on the security profile manager
	if len(m.activeDumps) >= m.config.RuntimeSecurity.ActivityDumpTracedCgroupsCount {
		return
	}

	// fetch silent workloads
workloadLoop:
	for selector, workloads := range m.fetchSilentWorkloads() {
		if len(workloads) == 0 {
			// this profile is on its way out, ignore
			continue
		}

		if len(m.activeDumps) >= m.config.RuntimeSecurity.ActivityDumpTracedCgroupsCount {
			// we're at capacity, ignore for now
			break
		}

		// check if we already have an activity dump for this selector
		for _, ad := range m.activeDumps {
			// the dump selector is resolved if it has been counted by the limiter
			if !ad.IsCountedByLimiter() {
				continue
			}

			adSelector := ad.Profile.GetWorkloadSelector()
			if adSelector != nil && adSelector.Match(selector) {
				// we already have an activity dump for this selector, ignore
				continue workloadLoop
			}
		}

		// if we're still here, we can start tracing this workload
		defaultConfigs, err := m.getDefaultLoadConfigs()
		if err != nil {
			seclog.Errorf("couldn't get default load configs: %v", err)
			continue
		}

		defaultConfig, found := defaultConfigs[containerutils.CGroupManager(workloads[0].CGroupContext.CGroupFlags)]
		if !found {
			seclog.Errorf("Failed to find default activity dump config for %s", containerutils.CGroupManager(workloads[0].CGroupContext.CGroupFlags).String())
			continue
		}

		if err := m.startDumpWithConfig(workloads[0].ContainerID, workloads[0].CGroupContext, utils.NewCookie(), *defaultConfig); err != nil {
			if !errors.Is(err, unix.E2BIG) {
				seclog.Debugf("%v", err)
				break
			}
			seclog.Errorf("%v", err)
		}
	}
}

func (m *Manager) startDumpWithConfig(containerID containerutils.ContainerID, cgroupContext model.CGroupContext, cookie uint64, loadConfig model.ActivityDumpLoadConfig) error {
	// create a new activity dump
	newDump := dump.NewActivityDump(m.pathsReducer, m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs, 0, m.config.RuntimeSecurity.ActivityDumpTracedEventTypes, m.updateTracedPid, &loadConfig, func(ad *dump.ActivityDump) {
		ad.Profile.Metadata.ContainerID = containerID
		ad.Profile.Metadata = mtdt.Metadata{
			AgentVersion:      version.AgentVersion,
			AgentCommit:       version.Commit,
			KernelVersion:     m.kernelVersion.Code.String(),
			LinuxDistribution: m.kernelVersion.OsRelease["PRETTY_NAME"],
			Arch:              utils.RuntimeArch(),

			Name:              fmt.Sprintf("activity-dump-%s", utils.RandString(10)),
			ProtobufVersion:   profile.ProtobufVersion,
			DifferentiateArgs: m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs,
			ContainerID:       containerID,
			CGroupContext:     cgroupContext,
			Start:             m.resolvers.TimeResolver.ResolveMonotonicTimestamp(loadConfig.StartTimestampRaw),
			End:               m.resolvers.TimeResolver.ResolveMonotonicTimestamp(loadConfig.EndTimestampRaw),
		}
		ad.Profile.Header.Host = m.hostname
		ad.Profile.Header.Source = ActivityDumpSource
	})
	newDump.Cookie = cookie

	if err := m.insertActivityDump(newDump); err != nil {
		return fmt.Errorf("couldn't start tracing [%s]: %v", newDump.GetSelectorStr(), err)
	}
	return nil
}

// HandleCGroupTracingEvent handles a cgroup tracing event
func (m *Manager) HandleCGroupTracingEvent(event *model.CgroupTracingEvent) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return
	}

	if len(event.CGroupContext.CGroupID) == 0 {
		seclog.Warnf("received a cgroup tracing event with an empty cgroup ID")
		return
	}

	m.m.Lock()
	defer m.m.Unlock()

	if err := m.startDumpWithConfig(event.ContainerContext.ContainerID, event.CGroupContext, event.ConfigCookie, event.Config); err != nil {
		seclog.Warnf("%v", err)
	}
}

// event lost recovery

// SnapshotTracedCgroups recovers lost CGroup tracing events by going through the kernel space map of cgroups
func (m *Manager) SnapshotTracedCgroups() {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return
	}

	var err error
	var event model.CgroupTracingEvent
	var cgroupFile model.PathKey
	iterator := m.tracedCgroupsMap.Iterate()
	seclog.Infof("snapshotting traced_cgroups map")

	for iterator.Next(&cgroupFile, &event.ConfigCookie) {
		m.m.Lock()
		if m.ignoreFromSnapshot[cgroupFile] {
			m.m.Unlock()
			continue
		}
		m.m.Unlock()

		if err = m.activityDumpsConfigMap.Lookup(&event.ConfigCookie, &event.Config); err != nil {
			// this config doesn't exist anymore, remove expired entries
			seclog.Warnf("config not found for (%v): %v", cgroupFile, err)
			_ = m.tracedCgroupsMap.Delete(cgroupFile)
			continue
		}

		cgroupContext, _, err := m.resolvers.ResolveCGroupContext(cgroupFile, event.Config.CGroupFlags)
		if err != nil {
			seclog.Warnf("couldn't resolve cgroup context for (%v): %v", cgroupFile, err)
			continue
		}
		event.CGroupContext = *cgroupContext

		m.HandleCGroupTracingEvent(&event)
	}

	if err = iterator.Err(); err != nil {
		seclog.Warnf("couldn't iterate over the map traced_cgroups: %v", err)
	}
}

// snapshot

func (m *Manager) newProcessCacheEntrySearcher(ad *dump.ActivityDump) *processCacheEntrySearcher {
	return &processCacheEntrySearcher{
		manager:       m,
		ad:            ad,
		ancestorCache: make(map[*model.ProcessContext]*model.ProcessCacheEntry),
	}
}

// updateTracedPid traces a pid in kernel space
func (m *Manager) updateTracedPid(ad *dump.ActivityDump, pid uint32) {
	// start by looking up any existing entry
	var cookie uint64
	_ = m.tracedPIDsMap.Lookup(pid, &cookie)
	if cookie != ad.Cookie {
		config := ad.LoadConfig.Load()
		_ = m.tracedPIDsMap.Put(pid, &config)
	}
}

type processCacheEntrySearcher struct {
	manager       *Manager
	ad            *dump.ActivityDump
	ancestorCache map[*model.ProcessContext]*model.ProcessCacheEntry
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
func (pces *processCacheEntrySearcher) searchTracedProcessCacheEntry(entry *model.ProcessCacheEntry) {
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

	pces.ad.Profile.AddSnapshotAncestors(
		ancestors,
		pces.manager.resolvers,
		func(pce *model.ProcessCacheEntry) {
			pces.manager.updateTracedPid(pces.ad, pce.Process.Pid)
		},
	)
}

// SendStats

// SendStats sends the manager stats
func (m *Manager) SendStats() error {
	m.m.Lock()
	defer m.m.Unlock()

	// ActivityDump stats
	if m.config.RuntimeSecurity.ActivityDumpEnabled {
		for _, ad := range m.activeDumps {
			if err := ad.Profile.SendStats(m.statsdClient); err != nil {
				return fmt.Errorf("couldn't send metrics for [%s]: %w", ad.GetSelectorStr(), err)
			}
		}

		activeDumps := float64(len(m.activeDumps))
		if err := m.statsdClient.Gauge(metrics.MetricActivityDumpActiveDumps, activeDumps, []string{}, 1.0); err != nil {
			seclog.Errorf("couldn't send MetricActivityDumpActiveDumps metric: %v", err)
		}

		if value := m.emptyDropped.Swap(0); value > 0 {
			if err := m.statsdClient.Count(metrics.MetricActivityDumpEmptyDropped, int64(value), nil, 1.0); err != nil {
				return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpEmptyDropped, err)
			}
		}

		if value := m.dropMaxDumpReached.Swap(0); value > 0 {
			if err := m.statsdClient.Count(metrics.MetricActivityDumpDropMaxDumpReached, int64(value), nil, 1.0); err != nil {
				return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpDropMaxDumpReached, err)
			}
		}

		if value := m.workloadDenyListHits.Swap(0); value > 0 {
			if err := m.statsdClient.Count(metrics.MetricActivityDumpWorkloadDenyListHits, int64(value), nil, 1.0); err != nil {
				return fmt.Errorf("couldn't send %s metric: %w", metrics.MetricActivityDumpWorkloadDenyListHits, err)
			}
		}

		m.localStorage.SendTelemetry(m.statsdClient)
		m.remoteStorage.SendTelemetry(m.statsdClient)
	}

	// SecProfile stats
	if m.config.RuntimeSecurity.SecurityProfileEnabled {
		m.profilesLock.Lock()
		defer m.profilesLock.Unlock()
		m.pendingCacheLock.Lock()
		defer m.pendingCacheLock.Unlock()

		profilesLoadedInKernel := 0
		profileVersions := make(map[string]int)
		for selector, profile := range m.profiles {
			if profile.LoadedInKernel.Load() { // make sure the profile is loaded
				profileVersions[selector.Image] = len(profile.Instances)
				if err := profile.SendStats(m.statsdClient); err != nil {
					return fmt.Errorf("couldn't send metrics for [%s]: %w", profile.GetSelectorStr(), err)
				}
				profilesLoadedInKernel++
			}
		}

		for imageName, nbVersions := range profileVersions {
			if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileVersions, float64(nbVersions), []string{"security_profile_image_name:" + imageName}, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricSecurityProfileVersions: %w", err)
			}
		}

		t := []string{
			fmt.Sprintf("in_kernel:%v", profilesLoadedInKernel),
		}
		if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileProfiles, float64(len(m.profiles)), t, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSecurityProfileProfiles: %w", err)
		}

		if val := float64(m.pendingCache.Len()); val > 0 {
			if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileCacheLen, val, []string{}, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricSecurityProfileCacheLen: %w", err)
			}
		}

		if val := int64(m.cacheHit.Swap(0)); val > 0 {
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileCacheHit, val, []string{}, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricSecurityProfileCacheHit: %w", err)
			}
		}

		if val := int64(m.cacheMiss.Swap(0)); val > 0 {
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileCacheMiss, val, []string{}, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricSecurityProfileCacheMiss: %w", err)
			}
		}

		for entry, count := range m.eventFiltering {
			t := []string{fmt.Sprintf("event_type:%s", entry.eventType), entry.state.ToTag(), entry.result.toTag()}
			if value := count.Swap(0); value > 0 {
				if err := m.statsdClient.Count(metrics.MetricSecurityProfileEventFiltering, int64(value), t, 1.0); err != nil {
					return fmt.Errorf("couldn't send MetricSecurityProfileEventFiltering metric: %w", err)
				}
			}
		}

		m.evictedVersionsLock.Lock()
		evictedVersions := m.evictedVersions
		m.evictedVersions = []cgroupModel.WorkloadSelector{}
		m.evictedVersionsLock.Unlock()
		for _, version := range evictedVersions {
			t := version.ToTags()
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileEvictedVersions, 1, t, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricSecurityProfileEvictedVersions metric: %w", err)
			}

		}
	}

	return nil
}

///////////// SecurityProfileManager

// fetchSilentWorkloads returns the list of workloads for which we haven't received any profile
func (m *Manager) fetchSilentWorkloads() map[cgroupModel.WorkloadSelector][]*tags.Workload {

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	out := make(map[cgroupModel.WorkloadSelector][]*tags.Workload)

	for selector, profile := range m.profiles {
		if !profile.LoadedInKernel.Load() {
			profile.InstancesLock.Lock()
			instances := make([]*tags.Workload, len(profile.Instances))
			copy(instances, profile.Instances)
			profile.InstancesLock.Unlock()
			out[selector] = instances
		}
	}

	return out
}

// LookupEventInProfiles lookups event in profiles
func (m *Manager) LookupEventInProfiles(event *model.Event) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return
	}

	// ignore events with an error
	if event.Error != nil {
		return
	}

	// create profile selector
	event.FieldHandlers.ResolveContainerTags(event, event.ContainerContext)
	if len(event.ContainerContext.Tags) == 0 {
		return
	}
	selector, err := cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", event.ContainerContext.Tags), "*")
	if err != nil {
		return
	}

	// lookup profile
	m.profilesLock.Lock()
	profile := m.profiles[selector]
	m.profilesLock.Unlock()
	if profile == nil {
		m.incrementEventFilteringStat(event.GetEventType(), model.NoProfile, NA)
		return
	}
	if !profile.IsEventTypeValid(event.GetEventType()) || !profile.LoadedInKernel.Load() {
		m.incrementEventFilteringStat(event.GetEventType(), model.NoProfile, NA)
		return
	}

	_ = event.FieldHandlers.ResolveContainerCreatedAt(event, event.ContainerContext)

	// check if the event should be injected in the profile automatically
	imageTag := utils.GetTagValue("image_tag", event.ContainerContext.Tags)
	if imageTag == "" {
		imageTag = "latest" // not sure about this one
	}

	ctx, found := profile.GetVersionContext(imageTag)
	if found {
		ctx.LastSeenNano = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now()))
	} else {
		evictedVersions := profile.PrepareNewVersion(imageTag, event.ContainerContext.Tags, m.config.RuntimeSecurity.SecurityProfileMaxImageTags, uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now())))
		for _, evictedVersion := range evictedVersions {
			m.countEvictedVersion(imageTag, evictedVersion)
		}
		ctx, found = profile.GetVersionContext(imageTag)
		if !found {
			return
		}
	}

	// if we have one version of the profile in unstable for this event type, just skip the whole process
	globalEventTypeProfilState := profile.GetGlobalEventTypeState(event.GetEventType())
	if globalEventTypeProfilState == model.UnstableEventType {
		m.incrementEventFilteringStat(event.GetEventType(), model.UnstableEventType, NA)
		// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
		// the anomaly flag if the user space profile considers it to not be an anomaly. Here, when a version is unstable,
		// we don't want to generate anomalies for this profile anymore.
		event.ResetAnomalyDetectionEvent()
		return
	}

	profileState := m.tryAutolearn(profile, ctx, event, imageTag)
	if profileState != model.NoProfile {
		ctx.EventTypeState[event.GetEventType()].State = profileState
	}
	switch profileState {
	case model.NoProfile, model.ProfileAtMaxSize, model.UnstableEventType:
		// an error occurred or we are in unstable state
		// do not link the profile to avoid sending anomalies

		// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
		// the anomaly flag if the user space profile considers it to not be an anomaly.
		// We can also get a syscall anomaly detection kernel space for runc, which is ignored in the activity tree
		// (i.e. tryAutolearn returns NoProfile) because "runc" can't be a root node.
		event.ResetAnomalyDetectionEvent()

		return
	case model.AutoLearning, model.WorkloadWarmup:
		// the event was either already in the profile, or has just been inserted
		fillProfileContextFromProfile(&event.SecurityProfileContext, profile, imageTag, profileState)
		event.AddToFlags(model.EventFlagsSecurityProfileInProfile)

		return
	case model.StableEventType:
		// check if the event is in its profile
		// and if this is not an exec event, check if we can benefit of the occasion to add missing processes
		insertMissingProcesses := false
		if event.GetEventType() != model.ExecEventType {
			if execState := m.getEventTypeState(profile, ctx, event, model.ExecEventType, imageTag); execState == model.AutoLearning || execState == model.WorkloadWarmup {
				insertMissingProcesses = true
			}
		}
		found, err := profile.Contains(event, insertMissingProcesses, imageTag, activity_tree.ProfileDrift, m.resolvers)
		if err != nil {
			// ignore, evaluation failed
			m.incrementEventFilteringStat(event.GetEventType(), model.NoProfile, NA)

			// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
			// the anomaly flag if the user space profile considers it to not be an anomaly.
			event.ResetAnomalyDetectionEvent()
			return
		}
		fillProfileContextFromProfile(&event.SecurityProfileContext, profile, imageTag, profileState)
		if found {
			event.AddToFlags(model.EventFlagsSecurityProfileInProfile)
			m.incrementEventFilteringStat(event.GetEventType(), profileState, InProfile)

			// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
			// the anomaly flag if the user space profile considers it to not be an anomaly.
			event.ResetAnomalyDetectionEvent()
		} else {
			m.incrementEventFilteringStat(event.GetEventType(), profileState, NotInProfile)
			if m.canGenerateAnomaliesFor(event) {
				event.AddToFlags(model.EventFlagsAnomalyDetectionEvent)
			}
		}
	}
}

// tryAutolearn tries to autolearn the input event. It returns the profile state: stable, unstable, autolearning or workloadwarmup
func (m *Manager) tryAutolearn(profile *profile.Profile, ctx *profile.VersionContext, event *model.Event, imageTag string) model.EventFilteringProfileState {
	profileState := m.getEventTypeState(profile, ctx, event, event.GetEventType(), imageTag)
	var nodeType activity_tree.NodeGenerationType
	if profileState == model.AutoLearning {
		nodeType = activity_tree.ProfileDrift
	} else if profileState == model.WorkloadWarmup {
		nodeType = activity_tree.WorkloadWarmup
	} else { // Stable or Unstable state
		return profileState
	}

	// here we are either in AutoLearning or WorkloadWarmup
	// try to insert the event in the profile

	// defines if we want or not to insert missing processes
	insertMissingProcesses := false
	if event.GetEventType() == model.ExecEventType {
		insertMissingProcesses = true
	} else if execState := m.getEventTypeState(profile, ctx, event, model.ExecEventType, imageTag); execState == model.AutoLearning || execState == model.WorkloadWarmup {
		insertMissingProcesses = true
	}

	newEntry, err := profile.Insert(event, insertMissingProcesses, imageTag, nodeType, m.resolvers)
	if err != nil {
		m.incrementEventFilteringStat(event.GetEventType(), model.NoProfile, NA)
		return model.NoProfile
	} else if newEntry {
		eventState, ok := ctx.EventTypeState[event.GetEventType()]
		if ok { // should always be the case
			eventState.LastAnomalyNano = event.TimestampRaw
		}

		// if a previous version of this profile was stable for this event type,
		// and a new entry was added, trigger an anomaly detection
		globalEventTypeState := profile.GetGlobalEventTypeState(event.GetEventType())
		if globalEventTypeState == model.StableEventType && m.canGenerateAnomaliesFor(event) {
			event.AddToFlags(model.EventFlagsAnomalyDetectionEvent)
		} else {
			// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
			// the anomaly flag if the user space profile considers it to not be an anomaly: there is a new entry and no
			// previous version is in stable state.
			event.ResetAnomalyDetectionEvent()
		}

		m.incrementEventFilteringStat(event.GetEventType(), profileState, NotInProfile)
	} else { // no newEntry
		m.incrementEventFilteringStat(event.GetEventType(), profileState, InProfile)
		// The anomaly flag can be set in kernel space by our eBPF programs (currently applies only to syscalls), reset
		// the anomaly flag if the user space profile considers it to not be an anomaly
		event.ResetAnomalyDetectionEvent()
	}
	return profileState
}

// fillProfileContextFromProfile fills the given ctx with profile infos
func fillProfileContextFromProfile(ctx *model.SecurityProfileContext, p *profile.Profile, imageTag string, state model.EventFilteringProfileState) {
	ctx.Name = p.Metadata.Name
	if ctx.Name == "" {
		ctx.Name = DefaultProfileName
	}

	ctx.EventTypes = p.GetEventTypes()
	ctx.EventTypeState = state
	profileContext, ok := p.GetVersionContext(imageTag)
	if ok { // should always be the case
		ctx.Tags = profileContext.Tags
	}
}

// FillProfileContextFromContainerID populates a SecurityProfileContext for the given container ID
func (m *Manager) FillProfileContextFromContainerID(id string, ctx *model.SecurityProfileContext, imageTag string) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return
	}

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for _, profile := range m.profiles {
		profile.InstancesLock.Lock()
		for _, instance := range profile.Instances {
			instance.Lock()
			if instance.ContainerID == containerutils.ContainerID(id) {
				ctx.Name = profile.Metadata.Name
				profileContext, ok := profile.GetVersionContext(imageTag)
				if ok { // should always be the case
					ctx.Tags = profileContext.Tags
				}
			}
			instance.Unlock()
		}
		profile.InstancesLock.Unlock()
	}
}

// loadProfile (thread unsafe) loads a Security Profile in kernel space
func (m *Manager) loadProfileMap(profile *profile.Profile) error {
	profile.LoadedInKernel.Store(true)
	profile.LoadedNano.Store(uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now())))

	// push kernel space filters
	if err := m.securityProfileSyscallsMap.Put(profile.GetProfileCookie(), profile.GenerateSyscallsFilters()); err != nil {
		return fmt.Errorf("couldn't push syscalls filter (check map size limit ?): %w", err)
	}

	// TODO: load generated programs
	seclog.Debugf("security profile %s loaded in kernel space", profile.Metadata.Name)
	return nil
}

// unloadProfile (thread unsafe) unloads a Security Profile from kernel space
func (m *Manager) unloadProfileMap(profile *profile.Profile) {
	profile.LoadedInKernel.Store(false)

	// remove kernel space filters
	if err := m.securityProfileSyscallsMap.Delete(profile.GetProfileCookie()); err != nil {
		seclog.Errorf("couldn't remove syscalls filter: %v", err)
	}

	// TODO: delete all kernel space programs
	seclog.Debugf("security profile %s unloaded from kernel space", profile.Metadata.Name)
}

// linkProfile (thread unsafe) updates the kernel space mapping between a workload and its profile
func (m *Manager) linkProfileMap(profile *profile.Profile, workload *tags.Workload) {
	if err := m.securityProfileMap.Put([]byte(workload.ContainerID), profile.GetProfileCookie()); err != nil {
		seclog.Errorf("couldn't link workload %s (selector: %s) with profile %s (check map size limit ?): %v", workload.ContainerID, workload.Selector.String(), profile.Metadata.Name, err)
		return
	}
	seclog.Infof("workload %s (selector: %s) successfully linked to profile %s", workload.ContainerID, workload.Selector.String(), profile.Metadata.Name)
}

// linkProfile applies a profile to the provided workload
func (m *Manager) linkProfile(profile *profile.Profile, workload *tags.Workload) {
	profile.InstancesLock.Lock()
	defer profile.InstancesLock.Unlock()

	// check if this instance of this workload is already tracked
	for _, w := range profile.Instances {
		if w.ContainerID == workload.ContainerID {
			// nothing to do, leave
			return
		}
	}

	// update the list of tracked instances
	profile.Instances = append(profile.Instances, workload)

	// can we apply the profile or is it not ready yet ?
	if profile.LoadedInKernel.Load() {
		m.linkProfileMap(profile, workload)
	}
}

// unlinkProfile (thread unsafe) updates the kernel space mapping between a workload and its profile
func (m *Manager) unlinkProfileMap(profile *profile.Profile, workload *tags.Workload) {
	if !profile.LoadedInKernel.Load() {
		return
	}

	if err := m.securityProfileMap.Delete([]byte(workload.ContainerID)); err != nil {
		seclog.Errorf("couldn't unlink workload %s (selector: %s) with profile %s: %v", workload.ContainerID, workload.Selector.String(), profile.Metadata.Name, err)
	}
	seclog.Infof("workload %s (selector: %s) successfully unlinked from profile %s", workload.ContainerID, workload.Selector.String(), profile.Metadata.Name)
}

// unlinkProfile removes the link between a workload and a profile
func (m *Manager) unlinkProfile(profile *profile.Profile, workload *tags.Workload) {
	profile.InstancesLock.Lock()
	defer profile.InstancesLock.Unlock()

	// remove the workload from the list of instances of the Security Profile
	for key, val := range profile.Instances {
		if workload.ContainerID == val.ContainerID {
			profile.Instances = append(profile.Instances[0:key], profile.Instances[key+1:]...)
			break
		}
	}

	// remove link between the profile and the workload
	m.unlinkProfileMap(profile, workload)
}

func (m *Manager) onWorkloadSelectorResolvedEvent(workload *tags.Workload) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	workload.Lock()
	defer workload.Unlock()

	if workload.Deleted.Load() {
		// this workload was deleted before we had time to apply its profile, ignore
		return
	}

	selector := workload.Selector
	selector.Tag = "*"

	// check if the workload of this selector already exists
	p, ok := m.profiles[selector]
	if !ok {
		// check the cache
		m.pendingCacheLock.Lock()
		defer m.pendingCacheLock.Unlock()
		p, ok = m.pendingCache.Get(selector)
		if ok {
			m.cacheHit.Inc()

			// remove profile from cache
			_ = m.pendingCache.Remove(selector)

			// since the profile was in cache, it was removed from kernel space, load it now
			// (locking isn't necessary here, but added as a safeguard)
			// TODO: check locking scheme
			err := m.loadProfileMap(p)
			if err != nil {
				seclog.Errorf("couldn't load security profile %s in kernel space: %v", p.GetSelectorStr(), err)
				return
			}

			// insert the profile in the list of active profiles
			m.profiles[selector] = p
		} else {
			m.cacheMiss.Inc()

			p = profile.New(
				profile.WithWorkloadSelector(selector),
				profile.WithPathsReducer(m.pathsReducer),
				profile.WithDifferentiateArgs(m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs),
				profile.WithDNSMatchMaxDepth(m.config.RuntimeSecurity.SecurityProfileDNSMatchMaxDepth),
				profile.WithEventTypes(m.secProfEventTypes),
			)

			// insert the profile in the list of active profiles
			m.profiles[selector] = p

			// try to load the profile from local storage
			ok, err := m.localStorage.Load(&selector, p)
			if err != nil {
				seclog.Warnf("couldn't load profile from local storage: %v", err)
				return
			} else if ok {
				err = m.loadProfileMap(p)
				if err != nil {
					seclog.Errorf("couldn't load security profile %s in kernel space: %v", p.GetSelectorStr(), err)
					return
				}
			}
		}
	}

	m.linkProfile(p, workload)
}

func (m *Manager) onWorkloadDeletedEvent(workload *tags.Workload) {
	// lookup the profile
	selector := cgroupModel.WorkloadSelector{
		Image: workload.Selector.Image,
		Tag:   "*",
	}
	m.profilesLock.Lock()
	p := m.profiles[selector]
	m.profilesLock.Unlock()
	if p == nil {
		// nothing to do, leave
		return
	}

	// removes the link between the profile and this workload
	m.unlinkProfile(p, workload)

	// check if the profile should be deleted
	m.shouldDeleteProfile(p)
}

func (m *Manager) shouldDeleteProfile(p *profile.Profile) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	m.pendingCacheLock.Lock()
	defer m.pendingCacheLock.Unlock()

	p.InstancesLock.Lock()
	defer p.InstancesLock.Unlock()
	// check if the profile should be deleted
	if len(p.Instances) != 0 {
		// this profile is still in use, leave now
		return
	}

	// remove the profile from the list of profiles
	delete(m.profiles, *p.GetWorkloadSelector())

	if p.LoadedInKernel.Load() {
		// remove profile from kernel space
		m.unloadProfileMap(p)
		if err := m.persistProfile(p); err != nil {
			seclog.Errorf("couldn't persist profile: %v", err)
		}
	}

	// cleanup profile before insertion in cache
	p.Reset()

	// TODO: is it possible to have a profile with no selector here?
	if p.GetWorkloadSelector().IsReady() {
		// do not insert in cache
		return
	}

	// add profile in cache
	m.pendingCache.Add(*p.GetWorkloadSelector(), p)
}

// persistProfile (thread unsafe) persists a profile to the filesystem
func (m *Manager) persistProfile(p *profile.Profile) error {
	raw, err := p.EncodeSecurityProfileProtobuf()
	if err != nil {
		return fmt.Errorf("couldn't encode profile: %w", err)
	}

	filename := p.Metadata.Name + ".profile"
	outputPath := path.Join(m.config.RuntimeSecurity.SecurityProfileDir, filename)
	tmpOutputPath := outputPath + ".tmp"

	// create output directory and output file, truncate existing file if a profile already exists
	err = os.MkdirAll(m.config.RuntimeSecurity.SecurityProfileDir, 0400)
	if err != nil {
		return fmt.Errorf("couldn't ensure directory [%s] exists: %w", m.config.RuntimeSecurity.SecurityProfileDir, err)
	}

	file, err := os.OpenFile(tmpOutputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0400)
	if err != nil {
		return fmt.Errorf("couldn't persist profile to file [%s]: %w", outputPath, err)
	}
	defer file.Close()

	if _, err := file.Write(raw.Bytes()); err != nil {
		return fmt.Errorf("couldn't write profile to file [%s]: %w", tmpOutputPath, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("error trying to close profile file [%s]: %w", file.Name(), err)
	}

	if err := os.Rename(tmpOutputPath, outputPath); err != nil {
		return fmt.Errorf("couldn't rename profile file [%s] to [%s]: %w", tmpOutputPath, outputPath, err)
	}

	seclog.Infof("[profile] file for %s written at: [%s]", p.GetSelectorStr(), outputPath)

	return nil
}

// onNewProfile handles the arrival of a new profile after it was created as an activity dump
func (m *Manager) onNewProfile(newProfile *profile.Profile) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	// here the new profile is coming from an activity dump, so its activity tree corresponds to the selector image_name + image_tag.
	// we need to update the selector to a security profile selector (image_name + "*" to match all image tags).
	selector := *newProfile.GetWorkloadSelector()
	profileManagerSelector := selector
	if selector.Tag != "*" {
		profileManagerSelector.Tag = "*"
	}

	// Update the Security Profile content
	p, ok := m.profiles[profileManagerSelector]
	if !ok {
		// this was likely a short-lived workload, cache the profile in case this workload comes back
		// insert in cache and leave
		p = profile.New(
			profile.WithWorkloadSelector(selector),
			profile.WithPathsReducer(m.pathsReducer),
			profile.WithDifferentiateArgs(m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs),
			profile.WithDNSMatchMaxDepth(m.config.RuntimeSecurity.SecurityProfileDNSMatchMaxDepth),
			profile.WithEventTypes(m.secProfEventTypes),
		)

		p.LoadFromNewProfile(newProfile)

		m.pendingCacheLock.Lock()
		defer m.pendingCacheLock.Unlock()
		m.pendingCache.Add(profileManagerSelector, p)
		return
	}

	// if profile was waited, push it
	if !p.LoadedInKernel.Load() {
		// merge the content of the new profile
		p.LoadFromNewProfile(newProfile)

		// load the profile in kernel space
		if err := m.loadProfileMap(p); err != nil {
			seclog.Errorf("couldn't load security profile %s in kernel space: %v", p.GetWorkloadSelector(), err)
			return
		}
		// link all workloads
		for _, workload := range p.Instances {
			m.linkProfileMap(p, workload)
		}
	}

	// if we already have a loaded profile for this workload, just ignore the new one
}

func (m *Manager) canGenerateAnomaliesFor(e *model.Event) bool {
	return m.config.RuntimeSecurity.AnomalyDetectionEnabled && slices.Contains(m.config.RuntimeSecurity.AnomalyDetectionEventTypes, e.GetEventType())
}

func (m *Manager) getEventTypeState(p *profile.Profile, pctx *profile.VersionContext, event *model.Event, eventType model.EventType, imageTag string) model.EventFilteringProfileState {
	eventState, ok := pctx.EventTypeState[event.GetEventType()]
	if !ok {
		eventState = &profile.EventTypeState{
			LastAnomalyNano: pctx.FirstSeenNano,
			State:           model.AutoLearning,
		}
		pctx.EventTypeState[eventType] = eventState
	} else if eventState.State == model.UnstableEventType {
		// If for the given event type we already are on UnstableEventType, just return
		// (once reached, this state is immutable)
		if eventType == event.GetEventType() { // increment stat only once for each event
			m.incrementEventFilteringStat(eventType, model.UnstableEventType, NA)
		}
		return model.UnstableEventType
	}

	var nodeType activity_tree.NodeGenerationType
	var profileState model.EventFilteringProfileState
	// check if we are at the beginning of a workload lifetime
	if event.ResolveEventTime().Sub(time.Unix(0, int64(event.ContainerContext.CreatedAt))) < m.config.RuntimeSecurity.AnomalyDetectionWorkloadWarmupPeriod {
		nodeType = activity_tree.WorkloadWarmup
		profileState = model.WorkloadWarmup
	} else {
		// If for the given event type we already are on StableEventType (and outside of the warmup period), just return
		if eventState.State == model.StableEventType {
			return model.StableEventType
		}

		if eventType == event.GetEventType() { // update the stable/unstable states only for the event event type
			// did we reached the stable state time limit ?
			if time.Duration(event.TimestampRaw-eventState.LastAnomalyNano) >= m.config.RuntimeSecurity.GetAnomalyDetectionMinimumStablePeriod(eventType) {
				eventState.State = model.StableEventType
				// call the activity dump manager to stop dumping workloads from the current profile selector
				if m.config.RuntimeSecurity.ActivityDumpEnabled {
					uniqueImageTagSeclector := *p.GetWorkloadSelector()
					uniqueImageTagSeclector.Tag = imageTag
					m.stopDumpsWithSelector(uniqueImageTagSeclector)
				}
				return model.StableEventType
			}

			// did we reached the unstable time limit ?
			if time.Duration(event.TimestampRaw-p.LoadedNano.Load()) >= m.config.RuntimeSecurity.AnomalyDetectionUnstableProfileTimeThreshold {
				eventState.State = model.UnstableEventType
				return model.UnstableEventType
			}
		}

		nodeType = activity_tree.ProfileDrift
		profileState = model.AutoLearning
	}

	// check if the unstable size limit was reached, but only for the event event type
	if eventType == event.GetEventType() && p.ComputeInMemorySize() >= m.config.RuntimeSecurity.AnomalyDetectionUnstableProfileSizeThreshold {
		// for each event type we want to reach either the StableEventType or UnstableEventType states, even
		// if we already reach the AnomalyDetectionUnstableProfileSizeThreshold. That's why we have to keep
		// rearming the lastAnomalyNano timer based on if it's something new or not.
		found, err := p.Contains(event, false /*insertMissingProcesses*/, imageTag, nodeType, m.resolvers)
		if err != nil {
			m.incrementEventFilteringStat(eventType, model.NoProfile, NA)
			return model.NoProfile
		} else if !found {
			eventState.LastAnomalyNano = event.TimestampRaw
		} else if profileState == model.WorkloadWarmup {
			// if it's NOT something's new AND we are on container warmup period, just pretend
			// we are in learning/warmup phase (as we know, this event is already present on the profile)
			return model.WorkloadWarmup
		}
		return model.ProfileAtMaxSize
	}
	return profileState
}

func (m *Manager) incrementEventFilteringStat(eventType model.EventType, state model.EventFilteringProfileState, result EventFilteringResult) {
	m.eventFiltering[eventFilteringEntry{eventType, state, result}].Inc()
}

// CountEvictedVersion count the evicted version for associated metric
func (m *Manager) countEvictedVersion(imageName, imageTag string) {
	m.evictedVersionsLock.Lock()
	defer m.evictedVersionsLock.Unlock()
	m.evictedVersions = append(m.evictedVersions, cgroupModel.WorkloadSelector{
		Image: imageName,
		Tag:   imageTag,
	})
}

func (m *Manager) persist(p *profile.Profile, formatsRequests map[config.StorageFormat][]config.StorageRequest) error {
	for format, requests := range formatsRequests {
		p.Metadata.Serialization = format.String()

		// encode profile
		data, err := p.Encode(format)
		if err != nil {
			seclog.Errorf("couldn't encode profile [%s] to %s format: %v", p.GetSelectorStr(), format, err)
			continue
		}

		for _, request := range requests {
			var storage storage.ActivityDumpStorage
			switch request.Type {
			case config.LocalStorage:
				storage = m.localStorage
			case config.RemoteStorage:
				storage = m.remoteStorage
			default:
				seclog.Errorf("couldn't persist [%s] to %s format: unknown storage type: %s", p.GetSelectorStr(), format, request.Type)
				continue
			}

			if err := storage.Persist(request, p, data); err != nil {
				seclog.Errorf("couldn't persist [%s] to %s storage: %v", p.GetSelectorStr(), request.Type, err)
			} else {
				tags := []string{"format:" + request.Format.String(), "storage_type:" + request.Type.String(), fmt.Sprintf("compression:%v", request.Compression)}
				if err := m.statsdClient.Count(metrics.MetricActivityDumpSizeInBytes, int64(data.Len()), tags, 1.0); err != nil {
					seclog.Warnf("couldn't send %s metric: %v", metrics.MetricActivityDumpSizeInBytes, err)
				}
				if err := m.statsdClient.Count(metrics.MetricActivityDumpPersistedDumps, 1, tags, 1.0); err != nil {
					seclog.Warnf("couldn't send %s metric: %v", metrics.MetricActivityDumpPersistedDumps, err)
				}
			}
		}
	}

	return nil
}

// ListAllProfileStates list all profiles and their versions (debug purpose only)
func (m *Manager) ListAllProfileStates() {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	for _, profile := range m.profiles {
		profile.ListAllVersionStates()
	}
}

// GetProfile returns a profile by its selector
func (m *Manager) GetProfile(selector cgroupModel.WorkloadSelector) *profile.Profile {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	// check if this workload had a Security Profile
	return m.profiles[selector]
}

// AddProfile adds a profile to the manager
// This function is only used for testing purposes
func (m *Manager) AddProfile(profile *profile.Profile) {
	m.newProfiles <- profile
}

func perFormatStorageRequests(requests []config.StorageRequest) map[config.StorageFormat][]config.StorageRequest {
	perFormatRequests := make(map[config.StorageFormat][]config.StorageRequest)
	for _, request := range requests {
		perFormatRequests[request.Format] = append(perFormatRequests[request.Format], request)
	}
	return perFormatRequests
}

///////////
// Called from gRPC Server gorountines
///////////

// ListActivityDumps returns the list of active activity dumps
func (m *Manager) ListActivityDumps(_ *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return &api.ActivityDumpListMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}

	m.m.Lock()
	defer m.m.Unlock()

	var activeDumpMsgs []*api.ActivityDumpMessage
	for _, d := range m.activeDumps {
		activeDumpMsgs = append(activeDumpMsgs, d.Profile.ToSecurityActivityDumpMessage(d.GetTimeout(), m.configuredStorageRequests))
	}
	return &api.ActivityDumpListMessage{
		Dumps: activeDumpMsgs,
	}, nil
}

// DumpActivity handles an activity dump request
func (m *Manager) DumpActivity(params *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return &api.ActivityDumpMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}

	if params.GetContainerID() == "" && params.GetCGroupID() == "" {
		err := errors.New("you must specify one selector between containerID and cgroupID")
		return &api.ActivityDumpMessage{Error: err.Error()}, err
	}

	var timeout time.Duration
	if params.GetTimeout() == "" {
		timeout = m.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout
	} else {
		var err error
		timeout, err = time.ParseDuration(params.GetTimeout())
		if err != nil {
			err := fmt.Errorf("failed to handle activity dump request: invalid timeout duration: %w", err)
			return &api.ActivityDumpMessage{Error: err.Error()}, err
		}
	}

	cgroupFlags := containerutils.CGroupFlags(0)
	if params.GetCGroupID() != "" {
		_, flags := containerutils.FindContainerID(containerutils.CGroupID(params.GetCGroupID()))
		cgroupFlags = containerutils.CGroupFlags(flags)
	}

	m.m.Lock()
	defer m.m.Unlock()

	now := time.Now()
	loadConfig := m.newActivityDumpLoadConfig(
		m.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		timeout,
		m.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout,
		m.config.RuntimeSecurity.ActivityDumpRateLimiter,
		now,
		cgroupFlags,
	)

	newDump := dump.NewActivityDump(m.pathsReducer, params.GetDifferentiateArgs(), 0, m.config.RuntimeSecurity.ActivityDumpTracedEventTypes, m.updateTracedPid, loadConfig, func(ad *dump.ActivityDump) {
		ad.Profile.Metadata = mtdt.Metadata{
			AgentVersion:      version.AgentVersion,
			AgentCommit:       version.Commit,
			KernelVersion:     m.kernelVersion.Code.String(),
			LinuxDistribution: m.kernelVersion.OsRelease["PRETTY_NAME"],
			Arch:              utils.RuntimeArch(),

			Name:              fmt.Sprintf("activity-dump-%s", utils.RandString(10)),
			ProtobufVersion:   profile.ProtobufVersion,
			DifferentiateArgs: params.GetDifferentiateArgs(),
			ContainerID:       containerutils.ContainerID(params.GetContainerID()),
			CGroupContext: model.CGroupContext{
				CGroupID: containerutils.CGroupID(params.GetCGroupID()),
			},
			Start: now,
			End:   now.Add(timeout),
		}
		ad.Profile.Header.Host = m.hostname
		ad.Profile.Header.Source = ActivityDumpSource
	})

	if err := m.insertActivityDump(newDump); err != nil {
		err := fmt.Errorf("couldn't start tracing [%s]: %w", params.GetContainerID(), err)
		return &api.ActivityDumpMessage{Error: err.Error()}, err
	}

	return newDump.Profile.ToSecurityActivityDumpMessage(timeout, m.configuredStorageRequests), nil
}

// StopActivityDump stops an active activity dump
func (m *Manager) StopActivityDump(params *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return &api.ActivityDumpStopMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}

	m.m.Lock()
	defer m.m.Unlock()

	if params.GetName() == "" && params.GetContainerID() == "" && params.GetCGroupID() == "" {
		err := errors.New("you must specify one selector between name, containerID and cgroupID")
		return &api.ActivityDumpStopMessage{Error: err.Error()}, err
	}

	toDelete := -1
	for i, ad := range m.activeDumps {
		if (params.GetName() != "" && ad.Profile.Metadata.Name == params.GetName()) ||
			(params.GetContainerID() != "" && ad.Profile.Metadata.ContainerID == containerutils.ContainerID(params.GetContainerID())) ||
			(params.GetCGroupID() != "" && ad.Profile.Metadata.CGroupContext.CGroupID == containerutils.CGroupID(params.GetCGroupID())) {
			m.finalizeKernelEventCollection(ad, true)
			seclog.Infof("tracing stopped for [%s]", ad.GetSelectorStr())
			toDelete = i

			// persist dump if not empty
			if !ad.Profile.IsEmpty() && ad.Profile.GetWorkloadSelector() != nil {
				if err := m.persist(ad.Profile, m.configuredStorageRequests); err != nil {
					seclog.Errorf("couldn't persist dump [%s]: %v", ad.GetSelectorStr(), err)
				} else if m.config.RuntimeSecurity.SecurityProfileEnabled { // drop the profile if we don't care about using it as a security profile
					select {
					case m.newProfiles <- ad.Profile:
					default:
						// drop the profile and log error if the channel is full
						seclog.Warnf("couldn't send new profile to the manager: channel is full")
					}
				}
			} else {
				m.emptyDropped.Inc()
			}
			break
		}
	}

	if toDelete >= 0 {
		m.activeDumps = append(m.activeDumps[:toDelete], m.activeDumps[toDelete+1:]...)
		return &api.ActivityDumpStopMessage{}, nil
	}

	var err error
	if params.GetName() != "" {
		err = fmt.Errorf("the activity dump manager does not contain any ActivityDump with the following name: %s", params.GetName())
	} else if params.GetContainerID() != "" {
		err = fmt.Errorf("the activity dump manager does not contain any ActivityDump with the following containerID: %s", params.GetContainerID())
	} else /* if params.GetCGroupID() != "" */ {
		err = fmt.Errorf("the activity dump manager does not contain any ActivityDump with the following cgroup ID: %s", params.GetCGroupID())
	}

	return &api.ActivityDumpStopMessage{Error: err.Error()}, err
}

// GenerateTranscoding executes the requested transcoding operation
func (m *Manager) GenerateTranscoding(params *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return &api.TranscodingRequestMessage{
			Error: ErrActivityDumpManagerDisabled.Error(),
		}, ErrActivityDumpManagerDisabled
	}

	m.m.Lock()
	defer m.m.Unlock()

	ad := dump.NewActivityDump(
		m.pathsReducer,
		m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs,
		0,
		m.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		m.updateTracedPid,
		m.defaultActivityDumpLoadConfig(time.Now(), containerutils.CGroupFlags(0)),
	)

	// open and parse input file
	if err := ad.Profile.Decode(params.GetActivityDumpFile()); err != nil {
		err := fmt.Errorf("couldn't parse input file %s: %w", params.GetActivityDumpFile(), err)
		return &api.TranscodingRequestMessage{Error: err.Error()}, err
	}

	// add transcoding requests
	storageRequests, err := config.ParseStorageRequests(params.GetStorage())
	if err != nil {
		err := fmt.Errorf("couldn't parse transcoding request for [%s]: %w", ad.GetSelectorStr(), err)
		return &api.TranscodingRequestMessage{Error: err.Error()}, err
	}

	if err := m.persist(ad.Profile, perFormatStorageRequests(storageRequests)); err != nil {
		err := fmt.Errorf("couldn't persist dump [%s]: %w", ad.GetSelectorStr(), err)
		return &api.TranscodingRequestMessage{Error: err.Error()}, err
	}

	message := &api.TranscodingRequestMessage{}
	for _, request := range storageRequests {
		message.Storage = append(message.Storage, request.ToStorageRequestMessage(ad.Profile.Metadata.Name))
	}

	return message, nil
}

// ListSecurityProfiles returns the list of security profiles
func (m *Manager) ListSecurityProfiles(params *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return &api.SecurityProfileListMessage{
			Error: ErrSecurityProfileManagerDisabled.Error(),
		}, ErrSecurityProfileManagerDisabled
	}

	var out api.SecurityProfileListMessage

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for _, p := range m.profiles {
		msg := p.ToSecurityProfileMessage(m.resolvers.TimeResolver)
		out.Profiles = append(out.Profiles, msg)
	}

	if params.GetIncludeCache() {
		m.pendingCacheLock.Lock()
		defer m.pendingCacheLock.Unlock()
		for _, k := range m.pendingCache.Keys() {
			p, ok := m.pendingCache.Peek(k)
			if !ok {
				continue
			}
			msg := p.ToSecurityProfileMessage(m.resolvers.TimeResolver)
			out.Profiles = append(out.Profiles, msg)
		}
	}
	return &out, nil
}

// SaveSecurityProfile saves the requested security profile to disk
func (m *Manager) SaveSecurityProfile(params *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return &api.SecurityProfileSaveMessage{
			Error: ErrSecurityProfileManagerDisabled.Error(),
		}, ErrSecurityProfileManagerDisabled
	}

	selector, err := cgroupModel.NewWorkloadSelector(params.GetSelector().GetName(), "*")
	if err != nil {
		return &api.SecurityProfileSaveMessage{
			Error: err.Error(),
		}, nil
	}

	m.profilesLock.Lock()
	p := m.profiles[selector]
	m.profilesLock.Unlock()

	if p == nil {
		return &api.SecurityProfileSaveMessage{
			Error: "security profile not found",
		}, nil
	}

	// encode profile
	raw, err := p.EncodeSecurityProfileProtobuf()
	if err != nil {
		return &api.SecurityProfileSaveMessage{
			Error: fmt.Sprintf("couldn't encode security profile in %s format: %v", config.Protobuf, err),
		}, nil
	}

	// write profile to encoded profile to disk
	f, err := os.CreateTemp("/tmp", fmt.Sprintf("%s-*.profile", p.Metadata.Name))
	if err != nil {
		return nil, fmt.Errorf("couldn't create temporary file: %w", err)
	}
	defer f.Close()

	if _, err = f.Write(raw.Bytes()); err != nil {
		return nil, fmt.Errorf("couldn't write to temporary file: %w", err)
	}

	return &api.SecurityProfileSaveMessage{
		File: f.Name(),
	}, nil
}
