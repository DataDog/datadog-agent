// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"context"
	"fmt"
	"os"
	"path"
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

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/hostnameutils"
)

const (
	// ActivityDumpSource defines the source of activity dumps
	ActivityDumpSource = "runtime-security-agent"
	// DefaultProfileName used as default profile name
	DefaultProfileName         = "default"
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
func NewManager(cfg *config.Config, statsdClient statsd.ClientInterface, ebpf *ebpfmanager.Manager, resolvers *resolvers.EBPFResolvers, kernelVersion *kernel.Version, newEvent func() *model.Event, dumpHandler backend.ActivityDumpHandler, ipc ipc.Component) (*Manager, error) {
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
	// the actual fields are not really used, but this allows to report the correct request
	configuredStorageRequests = append(configuredStorageRequests, config.NewStorageRequest(
		config.RemoteStorage,
		config.Protobuf,
		true, // force remote compression
		"",
	))

	hostname, err := hostnameutils.GetHostname(ipc)
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

func perFormatStorageRequests(requests []config.StorageRequest) map[config.StorageFormat][]config.StorageRequest {
	perFormatRequests := make(map[config.StorageFormat][]config.StorageRequest)
	for _, request := range requests {
		perFormatRequests[request.Format] = append(perFormatRequests[request.Format], request)
	}
	return perFormatRequests
}
