// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/hash"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
)

// newTestManagerV2WithLocalStorage builds a minimal ManagerV2 wired to a real on-disk local
// storage backend, configured to persist profiles in the security-profile format (the format the
// reload path reads back). Only the fields used by persistProfile are populated.
func newTestManagerV2WithLocalStorage(t *testing.T) (*ManagerV2, string) {
	t.Helper()

	dir := t.TempDir()
	localStorage, err := storage.NewDirectory(dir, 100)
	require.NoError(t, err)

	m := &ManagerV2{
		statsdClient: &statsd.NoOpClient{},
		localStorage: localStorage,
		configuredStorageRequests: perFormatStorageRequests([]config.StorageRequest{
			config.NewStorageRequest(config.LocalStorage, config.Profile, false, dir),
		}),
	}
	return m, dir
}

func newTestProfileWithNodes(name string, nodeCount int) *profile.Profile {
	p := profile.New(profile.WithWorkloadSelector(cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}))
	p.Metadata = mtdt.Metadata{Name: name}
	for i := 0; i < nodeCount; i++ {
		p.ActivityTree.ProcessNodes = append(p.ActivityTree.ProcessNodes, &activity_tree.ProcessNode{
			NodeBase: activity_tree.NewNodeBase(),
			Process: activity_tree.ProcessInfo{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/proc",
					BasenameStr: "proc",
				},
			},
		})
	}
	p.ActivityTree.ComputeActivityTreeStats()
	return p
}

// TestManagerV2_persistProfile_persistsDisabledState verifies that a profile disabled by the
// max-size safeguard is persisted to local storage carrying its disabled state, so it reloads as
// disabled after an Agent restart instead of coming back enabled and re-learning the workload.
func TestManagerV2_persistProfile_persistsDisabledState(t *testing.T) {
	selector := cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}

	// A profile disabled by the safeguard (tree dropped) must still be written to disk and must
	// reload as disabled.
	t.Run("persists a disabled profile and reloads it as disabled", func(t *testing.T) {
		m, dir := newTestManagerV2WithLocalStorage(t)
		p := newTestProfileWithNodes("disabled", 8)
		p.Disable()
		require.False(t, p.IsEnabled())
		require.True(t, p.ActivityTree.IsEmpty())

		m.persistProfile(p)

		assert.False(t, p.HasAlreadyBeenSent(), "persisting a disabled profile must not open the send-gated learning window")
		filePath := filepath.Join(dir, "disabled."+config.Profile.String())
		_, err := os.Stat(filePath)
		require.NoError(t, err, "a disabled profile must be persisted so its state survives a restart")

		reloaded := profile.New(profile.WithWorkloadSelector(selector))
		ok, err := m.localStorage.Load(&selector, reloaded)
		require.NoError(t, err)
		require.True(t, ok, "the persisted disabled profile should be found on disk")
		assert.False(t, reloaded.IsEnabled(), "a profile persisted as disabled must reload as disabled")
	})

	// An enabled profile keeps being persisted with its tree and reloads as enabled.
	t.Run("persists an enabled profile and reloads it as enabled", func(t *testing.T) {
		m, _ := newTestManagerV2WithLocalStorage(t)
		p := newTestProfileWithNodes("enabled", 8)
		require.True(t, p.IsEnabled())

		m.persistProfile(p)

		reloaded := profile.New(profile.WithWorkloadSelector(selector))
		ok, err := m.localStorage.Load(&selector, reloaded)
		require.NoError(t, err)
		require.True(t, ok, "the persisted enabled profile should be found on disk")
		assert.True(t, reloaded.IsEnabled(), "an enabled profile must reload as enabled")
		assert.False(t, reloaded.ActivityTree.IsEmpty(), "an enabled profile keeps its tree")
		assert.True(t, p.HasAlreadyBeenSent(), "persisting an enabled profile ends the send-gated learning window")
	})
}

// TestManagerV2_evictUnusedNodes_skipsDisabledProfile verifies the eviction tick leaves disabled
// profiles untouched: Disable() already empties the activity tree, so there is nothing to evict,
// and re-enabling a disabled profile is owned by a separate path, not the eviction loop.
func TestManagerV2_evictUnusedNodes_skipsDisabledProfile(t *testing.T) {
	cgr, err := cgroup.NewResolver(&statsd.NoOpClient{}, nil, nil)
	require.NoError(t, err)

	maxSize := 1 << 20
	m := &ManagerV2{
		statsdClient:         &statsd.NoOpClient{},
		resolvers:            &resolvers.EBPFResolvers{CGroupResolver: cgr},
		profiles:             make(map[cgroupModel.WorkloadSelector]*profile.Profile),
		evictionRuns:         atomic.NewUint64(0),
		evictionNodesEvicted: atomic.NewUint64(0),
		config: &config.Config{
			RuntimeSecurity: &config.RuntimeSecurityConfig{
				SecurityProfileNodeEvictionTimeout: time.Hour,
				ActivityDumpTraceSystemdCgroups:    false,
				SecurityProfileV2MaxDumpSize:       func() int { return maxSize },
			},
		},
	}

	selector := cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}
	p := newTestProfileWithNodes("over-limit", 8)
	// The max-size safeguard disables the profile and drops its tree.
	p.Disable()
	require.False(t, p.IsEnabled())
	require.True(t, p.ActivityTree.IsEmpty())
	m.profiles[selector] = p

	m.evictUnusedNodes()

	assert.False(t, p.IsEnabled(), "eviction must not re-enable a disabled profile")
}

func TestManagerV2_shouldSendAnomalyDetection(t *testing.T) {
	start := time.Now()
	withStart := func() *profile.Profile {
		p := profile.New()
		p.Metadata = mtdt.Metadata{Start: start}
		return p
	}
	timeBased := func(period time.Duration) *ManagerV2 {
		return &ManagerV2{config: &config.Config{RuntimeSecurity: &config.RuntimeSecurityConfig{
			SecurityProfileV2ProfileReportingDelayTimeBased: true,
			SecurityProfileV2ProfileReportingDelayDuration:  period,
		}}}
	}

	t.Run("default withholds until the profile has been persisted", func(t *testing.T) {
		p := withStart()
		m := &ManagerV2{config: &config.Config{RuntimeSecurity: &config.RuntimeSecurityConfig{}}}
		assert.False(t, m.shouldSendAnomalyDetection(p, start))
		p.SetHasAlreadyBeenSent()
		assert.True(t, m.shouldSendAnomalyDetection(p, start))
	})

	t.Run("time-based with a zero period sends as soon as the profile starts", func(t *testing.T) {
		p := withStart()
		assert.True(t, timeBased(0).shouldSendAnomalyDetection(p, start))
		assert.False(t, p.HasAlreadyBeenSent(), "time-based stabilization does not depend on persistence")
	})

	t.Run("time-based waits for the configured period after the profile starts", func(t *testing.T) {
		p := withStart()
		m := timeBased(time.Hour)
		assert.False(t, m.shouldSendAnomalyDetection(p, start))
		assert.False(t, m.shouldSendAnomalyDetection(p, start.Add(time.Hour-time.Nanosecond)))
		assert.True(t, m.shouldSendAnomalyDetection(p, start.Add(time.Hour)))
	})

	t.Run("time-based anchors on the persisted start, not the in-memory creation time", func(t *testing.T) {
		p := withStart()
		p.Metadata.Start = start.Add(-time.Hour)
		assert.True(t, timeBased(time.Hour).shouldSendAnomalyDetection(p, start))
	})

	t.Run("time-based keeps withholding when the clock jumps backward", func(t *testing.T) {
		p := withStart()
		assert.False(t, timeBased(time.Hour).shouldSendAnomalyDetection(p, start.Add(-time.Hour)))
	})

	t.Run("time-based with an unset start sends immediately", func(t *testing.T) {
		p := profile.New()
		require.True(t, p.Metadata.Start.IsZero())
		assert.True(t, timeBased(time.Hour).shouldSendAnomalyDetection(p, time.Now()))
	})
}

func TestBaseMountNamespaceFromEvent(t *testing.T) {
	newProc := func(path, basename string, inode uint64, containerID string, mntns uint32) *model.ProcessCacheEntry {
		e := model.NewProcessCacheEntry()
		e.Process.FileEvent.PathnameStr = path
		e.Process.FileEvent.BasenameStr = basename
		e.Process.FileEvent.Inode = inode
		e.Process.FileEvent.MountID = 1 // non-zero so the file is not treated as fileless
		e.Process.ContainerContext.ContainerID = containerutils.ContainerID(containerID)
		e.Process.MntNS = mntns
		return e
	}

	eventFor := func(leaf *model.ProcessCacheEntry) *model.Event {
		ev := &model.Event{}
		ev.BaseEvent.ProcessContext = &leaf.ProcessContext
		return ev
	}

	t.Run("nil process context yields zero", func(t *testing.T) {
		nsID, authoritative := baseMountNamespaceFromEvent(&model.Event{})
		assert.Equal(t, uint32(0), nsID)
		assert.False(t, authoritative)
	})

	t.Run("falls back to the event process namespace when there is no valid root", func(t *testing.T) {
		p := newProc("/app/server", "server", 20, "c1", 4242)
		nsID, authoritative := baseMountNamespaceFromEvent(eventFor(p))
		assert.Equal(t, uint32(4242), nsID)
		// the branch is truncated, so the namespace is a guess and must stay revisable
		assert.False(t, authoritative)
	})

	t.Run("skips the container runtime and anchors on the workload root", func(t *testing.T) {
		shell := newProc("/bin/bash", "bash", 5, "", 1)
		runc := newProc("/usr/bin/runc", "runc", 10, "c1", 50)
		runc.Ancestor = shell
		app := newProc("/app/server", "server", 20, "c1", 100)
		app.Ancestor = runc
		// a sampled descendant that has setns'd into another namespace
		child := newProc("/app/child", "child", 30, "c1", 999)
		child.Ancestor = app

		nsID, authoritative := baseMountNamespaceFromEvent(eventFor(child))
		assert.Equal(t, uint32(100), nsID)
		assert.True(t, authoritative)
	})

	t.Run("does not cross the workload boundary into host processes", func(t *testing.T) {
		systemd := newProc("/usr/lib/systemd/systemd", "systemd", 1, "", 1)
		containerd := newProc("/usr/bin/containerd", "containerd", 5, "", 2)
		containerd.Ancestor = systemd
		shim := newProc("/usr/bin/containerd-shim", "containerd-shim", 10, "", 3)
		shim.Ancestor = containerd
		app := newProc("/app/server", "server", 20, "c1", 100)
		app.Ancestor = shim

		// the host containerd passes isValidRootNode, but it is outside the
		// workload and its host namespace must not be pinned as the base.
		nsID, authoritative := baseMountNamespaceFromEvent(eventFor(app))
		assert.Equal(t, uint32(100), nsID)
		assert.True(t, authoritative)
	})
}

func TestManagerV2_withinProfilingStartupDelay(t *testing.T) {
	const startMono = int64(time.Hour)
	newManager := func(delay time.Duration) *ManagerV2 {
		return &ManagerV2{
			startTimeMono: startMono,
			config: &config.Config{RuntimeSecurity: &config.RuntimeSecurityConfig{
				SecurityProfileV2ProfilingStartupDelay: delay,
			}},
		}
	}

	t.Run("disabled by default", func(t *testing.T) {
		assert.False(t, newManager(0).withinProfilingStartupDelay(uint64(startMono)))
	})

	t.Run("ignores events within the delay and resumes after it", func(t *testing.T) {
		m := newManager(time.Minute)
		assert.True(t, m.withinProfilingStartupDelay(uint64(startMono)))
		assert.True(t, m.withinProfilingStartupDelay(uint64(startMono+time.Minute.Nanoseconds()-1)))
		assert.False(t, m.withinProfilingStartupDelay(uint64(startMono+time.Minute.Nanoseconds())))
	})
}

func newTestManagerV2() (*ManagerV2, *lru.Cache[uint64, sampleCookieEntry]) {
	cookieMap, _ := lru.New[uint64, sampleCookieEntry](128)
	m := &ManagerV2{
		sampleCookieMap:       cookieMap,
		sampleRefreshReceived: atomic.NewUint64(0),
		sampleRefreshHits:     atomic.NewUint64(0),
		sampleRefreshMisses:   atomic.NewUint64(0),
	}
	return m, cookieMap
}

func TestManagerV2_HandleSampleRefresh(t *testing.T) {
	t.Run("unknown_cookie", func(t *testing.T) {
		m, _ := newTestManagerV2()
		m.HandleSampleRefresh(42)
		assert.Equal(t, uint64(1), m.sampleRefreshMisses.Load())
	})

	t.Run("valid_cookie_updates_process_and_event_node", func(t *testing.T) {
		m, cookieMap := newTestManagerV2()
		prof := profile.New()
		imageTagID := prof.ActivityTree.GetOrInsertImageTag("v1")

		processNode := &activity_tree.ProcessNode{}
		processNode.NodeBase = activity_tree.NewNodeBase()
		eventNodeBase := activity_tree.NewNodeBase()

		initialTime := time.Now().Add(-time.Hour)
		processNode.AppendImageTagID(imageTagID, initialTime)
		eventNodeBase.AppendImageTagID(imageTagID, initialTime)

		cookieMap.Add(uint64(1), sampleCookieEntry{
			profile:       prof,
			processNode:   processNode,
			eventNodeBase: &eventNodeBase,
			imageTag:      "v1",
		})

		m.HandleSampleRefresh(1)

		_, procLastSeen, ok := processNode.GetSeenTimes(imageTagID)
		assert.True(t, ok)
		assert.Greater(t, procLastSeen, initialTime.UnixNano())

		_, evtLastSeen, ok := eventNodeBase.GetSeenTimes(imageTagID)
		assert.True(t, ok)
		assert.Greater(t, evtLastSeen, initialTime.UnixNano())
	})

	t.Run("valid_cookie_nil_event_node_updates_process_only", func(t *testing.T) {
		m, cookieMap := newTestManagerV2()
		prof := profile.New()
		imageTagID := prof.ActivityTree.GetOrInsertImageTag("v1")

		processNode := &activity_tree.ProcessNode{}
		processNode.NodeBase = activity_tree.NewNodeBase()

		initialTime := time.Now().Add(-time.Hour)
		processNode.AppendImageTagID(imageTagID, initialTime)

		cookieMap.Add(uint64(1), sampleCookieEntry{
			profile:       prof,
			processNode:   processNode,
			eventNodeBase: nil,
			imageTag:      "v1",
		})

		m.HandleSampleRefresh(1)

		_, procLastSeen, ok := processNode.GetSeenTimes(imageTagID)
		assert.True(t, ok)
		assert.Greater(t, procLastSeen, initialTime.UnixNano())
	})

	t.Run("nil_process_node_removes_cookie", func(t *testing.T) {
		m, cookieMap := newTestManagerV2()
		prof := profile.New()

		cookieMap.Add(uint64(2), sampleCookieEntry{
			profile:     prof,
			processNode: nil,
			imageTag:    "v1",
		})

		m.HandleSampleRefresh(2)
		assert.False(t, cookieMap.Contains(uint64(2)))
	})

	t.Run("empty_seen_map_removes_cookie", func(t *testing.T) {
		m, cookieMap := newTestManagerV2()
		prof := profile.New()
		processNode := &activity_tree.ProcessNode{}
		processNode.NodeBase = activity_tree.NewNodeBase()

		cookieMap.Add(uint64(3), sampleCookieEntry{
			profile:     prof,
			processNode: processNode,
			imageTag:    "v1",
		})

		m.HandleSampleRefresh(3)
		assert.False(t, cookieMap.Contains(uint64(3)))
	})
}

const (
	testContainerID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testImageName   = "img"
)

var testSelector = cgroupModel.WorkloadSelector{Image: testImageName, Tag: "*"}

// fakeContainerTagger resolves the tags of the test container
type fakeContainerTagger struct {
	tags []string
}

func (f *fakeContainerTagger) Tag(entity types.EntityID, _ types.TagCardinality) ([]string, error) {
	if entity.GetID() != testContainerID {
		return nil, nil
	}
	return f.tags, nil
}

func (f *fakeContainerTagger) GlobalTags(_ types.TagCardinality) ([]string, error) {
	return nil, nil
}

// newInsertionTestManagerV2 builds a ManagerV2 able to insert events in profiles: profiles and anomaly
// detection are enabled, and anomalies are sent as soon as a profile starts. The returned slice collects the
// events sent as anomalies.
func newInsertionTestManagerV2(t *testing.T) (*ManagerV2, *[]*model.Event) {
	t.Helper()

	cgr, err := cgroup.NewResolver(&statsd.NoOpClient{}, nil, nil)
	require.NoError(t, err)
	// the profile only accepts the processes of the workloads linked to it, found in the cgroup cache
	require.NotNil(t, cgr.Add(model.CGroupContext{
		CGroupID:      containerutils.CGroupID("/docker/" + testContainerID),
		CGroupPathKey: model.PathKey{Inode: 4242, MountID: 1},
	}))
	timeResolver, err := ktime.NewResolver()
	require.NoError(t, err)
	localStorage, err := storage.NewDirectory(t.TempDir(), 100)
	require.NoError(t, err)
	cookieMap, err := lru.New[uint64, sampleCookieEntry](128)
	require.NoError(t, err)
	imgExcluder, err := newImageExcluder(nil)
	require.NoError(t, err)

	tagger := &fakeContainerTagger{tags: []string{"image_name:" + testImageName, "image_tag:v1"}}
	maxSize := 1 << 20

	var anomalies []*model.Event
	m := &ManagerV2{
		config: &config.Config{
			RuntimeSecurity: &config.RuntimeSecurityConfig{
				SecurityProfileEnabled:                          true,
				SecurityProfileV2Enabled:                        true,
				AnomalyDetectionEnabled:                         true,
				SecurityProfileV2EventTypes:                     []model.EventType{model.ExecEventType, model.FileOpenEventType, model.DNSEventType, model.BindEventType, model.ConnectEventType},
				SecurityProfileV2MaxDumpSize:                    func() int { return maxSize },
				SecurityProfileV2ProfileReportingDelayTimeBased: true,
				SecurityProfileDNSMatchMaxDepth:                 3,
				SecurityProfileCleanupDelay:                     time.Minute,
			},
		},
		statsdClient: &statsd.NoOpClient{},
		resolvers: &resolvers.EBPFResolvers{
			CGroupResolver: cgr,
			TagsResolver:   tags.NewResolver(0, tagger, cgr, nil),
			TimeResolver:   timeResolver,
			HashResolver:   &hash.Resolver{},
		},
		kernelVersion:          &kernel.Version{},
		pathsReducer:           activity_tree.NewPathsReducer(),
		profiles:               make(map[cgroupModel.WorkloadSelector]*profile.Profile),
		profilePendingEvents:   make(map[containerutils.CGroupID]*pendingProfile),
		queueSize:              atomic.NewUint64(0),
		pendingProfiles:        atomic.NewUint64(0),
		resolvedCgroups:        make(map[containerutils.CGroupID]struct{}),
		localStorage:           localStorage,
		eventFiltering:         make(map[eventFilteringEntry]*atomic.Uint64),
		insertionErrors:        make(map[insertionErrorKey]*atomic.Uint64),
		pendingProfileRemovals: make(map[cgroupModel.WorkloadSelector]time.Time),
		sampleCookieMap:        cookieMap,
		cleanupProfilesRemoved: atomic.NewUint64(0),
		imageExcluder:          imgExcluder,
		sendAnomalyDetection: func(event *model.Event) {
			anomalies = append(anomalies, event)
		},
	}
	m.initMetricsMap()
	return m, &anomalies
}

var testPorts = map[string]uint16{"known": 4251, "new": 4252}

// newTestEventV2 forges an event of the provided type, in the test container running the provided image tag.
// The same variant always produces the same activity, two variants produce two distinct profile entries.
func newTestEventV2(eventType model.EventType, imageTag string, variant string) *model.Event {
	event := model.NewFakeEvent()
	event.Type = uint32(eventType)
	event.Timestamp = time.Now()
	event.TimestampRaw = uint64(event.Timestamp.UnixNano())

	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	entry.ContainerContext.ContainerID = containerutils.ContainerID(testContainerID)
	entry.ContainerContext.Tags = []string{"image_name:" + testImageName, "image_tag:" + imageTag}
	entry.CGroup.CGroupID = containerutils.CGroupID("/docker/" + testContainerID)
	entry.CGroup.CGroupPathKey = model.PathKey{Inode: 4242, MountID: 1}
	entry.FileEvent.PathnameStr = "/usr/bin/proc"
	entry.FileEvent.BasenameStr = "proc"
	// a file with an inode but without mount ID would be seen as fileless
	entry.FileEvent.Inode = 42
	entry.FileEvent.MountID = 1
	entry.Ancestor = model.NewPlaceholderProcessCacheEntry(1, 1, false)
	entry.Ancestor.FileEvent.PathnameStr = "/usr/bin/containerd-shim-runc-v2"
	entry.Ancestor.FileEvent.BasenameStr = "containerd-shim-runc-v2"
	entry.Ancestor.FileEvent.Inode = 41
	entry.Ancestor.FileEvent.MountID = 1
	event.ProcessCacheEntry = entry
	event.ProcessContext = &entry.ProcessContext

	addr := model.IPPortContext{
		IPNet: net.IPNet{IP: net.IPv4zero.To4(), Mask: net.CIDRMask(32, 32)},
		Port:  testPorts[variant],
	}
	switch eventType {
	case model.ExecEventType:
		entry.FileEvent.PathnameStr = "/usr/bin/" + variant
		entry.FileEvent.BasenameStr = variant
		event.Exec.Process = &entry.Process
	case model.FileOpenEventType:
		event.Open.File.PathnameStr = "/tmp/" + variant
		event.Open.File.BasenameStr = variant
		event.Open.Flags = unix.O_RDONLY
	case model.DNSEventType:
		event.DNS.Question = model.DNSQuestion{Name: variant + "-domain.org", Type: 1, Class: 1, Size: 16, Count: 1}
	case model.BindEventType:
		event.Bind.Addr = addr
		event.Bind.AddrFamily = unix.AF_INET
		event.Bind.Protocol = unix.IPPROTO_UDP
	case model.ConnectEventType:
		event.Connect.Addr = addr
		event.Connect.AddrFamily = unix.AF_INET
		event.Connect.Protocol = unix.IPPROTO_UDP
	}
	return event
}

var testEventTypesV2 = []model.EventType{model.ExecEventType, model.FileOpenEventType, model.DNSEventType, model.BindEventType, model.ConnectEventType}

// findTestEventNode returns the profile node holding the activity of the variant, or nil
func findTestEventNode(p *profile.Profile, eventType model.EventType, variant string) *activity_tree.NodeBase {
	for _, pn := range p.ActivityTree.ProcessNodes {
		switch eventType {
		case model.ExecEventType:
			if pn.Process.FileEvent.PathnameStr == "/usr/bin/"+variant {
				return &pn.NodeBase
			}
		case model.FileOpenEventType:
			if tmp, ok := pn.Files["tmp"]; ok {
				if file, ok := tmp.Children[variant]; ok {
					return &file.NodeBase
				}
			}
		case model.DNSEventType:
			if dns, ok := pn.DNSNames[variant+"-domain.org"]; ok {
				return &dns.NodeBase
			}
		case model.BindEventType, model.ConnectEventType:
			for _, sock := range pn.Sockets {
				if eventType == model.BindEventType {
					for _, bind := range sock.Bind {
						if bind.Port == testPorts[variant] {
							return &bind.NodeBase
						}
					}
				} else {
					for _, connect := range sock.Connect {
						if connect.Port == testPorts[variant] {
							return &connect.NodeBase
						}
					}
				}
			}
		}
	}
	return nil
}

func TestManagerV2_insertEventIntoProfile(t *testing.T) {
	for _, eventType := range testEventTypesV2 {
		t.Run(eventType.String(), func(t *testing.T) {
			m, _ := newInsertionTestManagerV2(t)

			p, inserted := m.insertEventIntoProfile(newTestEventV2(eventType, "v1", "known"))
			require.NotNil(t, p)
			assert.True(t, inserted, "a first activity should create a new entry")
			assert.Equal(t, testSelector, *p.GetWorkloadSelector(), "the profile should be shared by all the tags of the image")
			node := findTestEventNode(p, eventType, "known")
			require.NotNil(t, node, "the %s node should be in the profile", eventType)
			_, _, ok := node.GetSeenTimes(p.ActivityTree.GetImageTagID("v1"))
			assert.True(t, ok, "the node should be tagged with v1")

			// the same activity under another tag of the image only adds the tag to the existing entry
			p2, inserted := m.insertEventIntoProfile(newTestEventV2(eventType, "v2", "known"))
			require.Same(t, p, p2)
			assert.False(t, inserted, "a known activity under another tag shouldn't create a new entry")
			assert.ElementsMatch(t, []string{"v1", "v2"}, p.GetVersions())
			node = findTestEventNode(p, eventType, "known")
			require.NotNil(t, node)
			_, _, ok = node.GetSeenTimes(p.ActivityTree.GetImageTagID("v2"))
			assert.True(t, ok, "the existing node should be tagged with v2")

			_, inserted = m.insertEventIntoProfile(newTestEventV2(eventType, "v2", "new"))
			assert.True(t, inserted, "a new activity should create a new entry")
			assert.NotNil(t, findTestEventNode(p, eventType, "new"))
		})
	}
}

func TestManagerV2_onEventTagsResolved(t *testing.T) {
	for _, eventType := range testEventTypesV2 {
		t.Run(eventType.String(), func(t *testing.T) {
			t.Run("new entries trigger an anomaly as soon as the profile starts", func(t *testing.T) {
				m, anomalies := newInsertionTestManagerV2(t)

				m.onEventTagsResolved(newTestEventV2(eventType, "v1", "known"))
				require.Len(t, *anomalies, 1)
				assert.Equal(t, eventType, (*anomalies)[0].GetEventType())
				assert.True(t, (*anomalies)[0].IsAnomalyDetectionEvent(), "the anomaly should be flagged as such")

				m.onEventTagsResolved(newTestEventV2(eventType, "v1", "known"))
				assert.Len(t, *anomalies, 1, "a known activity shouldn't trigger an anomaly")

				m.onEventTagsResolved(newTestEventV2(eventType, "v1", "new"))
				assert.Len(t, *anomalies, 2, "a new activity should trigger an anomaly")
			})

			t.Run("no anomaly before the first persistence by default", func(t *testing.T) {
				m, anomalies := newInsertionTestManagerV2(t)
				m.config.RuntimeSecurity.SecurityProfileV2ProfileReportingDelayTimeBased = false

				m.onEventTagsResolved(newTestEventV2(eventType, "v1", "known"))
				assert.Empty(t, *anomalies, "no anomaly should be sent before the first persistence")
				p := m.profiles[testSelector]
				require.NotNil(t, p)
				assert.NotNil(t, findTestEventNode(p, eventType, "known"), "the activity should still be learned")

				p.SetHasAlreadyBeenSent()
				m.onEventTagsResolved(newTestEventV2(eventType, "v1", "known"))
				assert.Empty(t, *anomalies, "an activity learned before the first persistence shouldn't trigger an anomaly")
				m.onEventTagsResolved(newTestEventV2(eventType, "v1", "new"))
				assert.Len(t, *anomalies, 1, "a new activity should trigger an anomaly after the first persistence")
			})

			t.Run("no anomaly when anomaly detection is disabled", func(t *testing.T) {
				m, anomalies := newInsertionTestManagerV2(t)
				m.config.RuntimeSecurity.AnomalyDetectionEnabled = false

				m.onEventTagsResolved(newTestEventV2(eventType, "v1", "known"))
				assert.Empty(t, *anomalies)
				p := m.profiles[testSelector]
				require.NotNil(t, p)
				assert.NotNil(t, findTestEventNode(p, eventType, "known"), "the activity should still be learned")
			})
		})
	}
}

func TestManagerV2_insertEventIntoProfile_maxSize(t *testing.T) {
	t.Run("a profile reaching the max size is disabled and stops learning", func(t *testing.T) {
		m, anomalies := newInsertionTestManagerV2(t)
		m.config.RuntimeSecurity.SecurityProfileV2MaxDumpSize = func() int { return 1 }

		// the size is checked before the insertion: an empty profile is below any max size
		p, inserted := m.insertEventIntoProfile(newTestEventV2(model.ExecEventType, "v1", "known"))
		require.NotNil(t, p)
		require.True(t, inserted)

		p, inserted = m.insertEventIntoProfile(newTestEventV2(model.ExecEventType, "v1", "new"))
		assert.Nil(t, p)
		assert.False(t, inserted)

		disabled := m.profiles[testSelector]
		require.NotNil(t, disabled, "the profile should be kept, disabled")
		assert.False(t, disabled.IsEnabled())
		assert.True(t, disabled.ActivityTree.IsEmpty(), "a disabled profile drops its activity tree")

		m.onEventTagsResolved(newTestEventV2(model.ExecEventType, "v1", "new"))
		assert.Empty(t, *anomalies, "a disabled profile shouldn't trigger anomalies")
		assert.True(t, disabled.ActivityTree.IsEmpty(), "a disabled profile shouldn't learn new activities")
		assert.Positive(t, m.eventFiltering[eventFilteringEntry{model.ExecEventType, model.ProfileAtMaxSize, NA}].Load())
	})

	t.Run("a profile below the max size keeps learning", func(t *testing.T) {
		m, _ := newInsertionTestManagerV2(t)

		p, inserted := m.insertEventIntoProfile(newTestEventV2(model.ExecEventType, "v1", "known"))
		require.NotNil(t, p)
		assert.True(t, inserted)
		assert.True(t, p.IsEnabled())
	})
}

func TestManagerV2_cleanupPendingProfiles(t *testing.T) {
	const cleanupDelay = time.Minute

	newInstance := func() *tags.Workload {
		cgce := cgroupModel.NewCacheEntry(model.ContainerContext{ContainerID: testContainerID}, model.CGroupContext{CGroupID: testContainerID}, 0)
		return &tags.Workload{GCroupCacheEntry: cgce, Selector: testSelector}
	}

	tests := []struct {
		name          string
		pendingSince  time.Duration
		hasInstance   bool
		expectRemoved bool
	}{
		{name: "removed once the cleanup delay is over", pendingSince: 2 * cleanupDelay, expectRemoved: true},
		{name: "kept until the cleanup delay is over", pendingSince: cleanupDelay / 2},
		{name: "kept when an instance came back", pendingSince: 2 * cleanupDelay, hasInstance: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newInsertionTestManagerV2(t)
			m.config.RuntimeSecurity.SecurityProfileCleanupDelay = cleanupDelay

			p := profile.New(profile.WithWorkloadSelector(testSelector))
			if tt.hasInstance {
				p.Instances = append(p.Instances, newInstance())
			}
			m.profiles[testSelector] = p
			m.pendingProfileRemovals[testSelector] = time.Now().Add(-tt.pendingSince)

			m.cleanupPendingProfiles()

			_, present := m.profiles[testSelector]
			assert.Equal(t, !tt.expectRemoved, present)
			if tt.expectRemoved {
				assert.NotContains(t, m.pendingProfileRemovals, testSelector)
				assert.Equal(t, uint64(1), m.cleanupProfilesRemoved.Load())
			}
		})
	}

	t.Run("a new event of the workload cancels the pending removal", func(t *testing.T) {
		m, _ := newInsertionTestManagerV2(t)
		m.config.RuntimeSecurity.SecurityProfileCleanupDelay = cleanupDelay

		_, inserted := m.insertEventIntoProfile(newTestEventV2(model.ExecEventType, "v1", "known"))
		require.True(t, inserted)
		m.pendingProfileRemovals[testSelector] = time.Now().Add(-2 * cleanupDelay)

		_, _ = m.insertEventIntoProfile(newTestEventV2(model.ExecEventType, "v1", "known"))
		assert.NotContains(t, m.pendingProfileRemovals, testSelector)

		m.cleanupPendingProfiles()
		assert.Contains(t, m.profiles, testSelector)
	})
}

type lazyTimeFieldHandlers struct {
	*model.FakeFieldHandlers
}

func (fh *lazyTimeFieldHandlers) ResolveEventTime(ev *model.Event, _ *model.BaseEvent) time.Time {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	return ev.Timestamp
}

func TestManagerV2_queueAndResolvePendingEvents(t *testing.T) {
	t.Run("queues event with unresolved timestamp and drains on periodic resolution", func(t *testing.T) {
		m, anomalies := newInsertionTestManagerV2(t)
		tagger := &fakeContainerTagger{}
		m.resolvers.TagsResolver = tags.NewResolver(0, tagger, m.resolvers.CGroupResolver, nil)

		ev := newTestEventV2(model.ExecEventType, "v1", "known")
		ev.Timestamp = time.Time{}
		ev.FieldHandlers = &lazyTimeFieldHandlers{FakeFieldHandlers: &model.FakeFieldHandlers{}}
		ev.ProcessContext.Process.ContainerContext.Tags = nil
		ev.Rules = []*model.MatchedRule{
			model.NewMatchedRule("rule_1", "1.0", nil, "pol", "1.0"),
		}

		m.queueEventForTagResolution(ev, nil)
		require.Equal(t, uint64(1), m.queueSize.Load())
		require.Equal(t, uint64(1), m.pendingProfiles.Load())

		tagger.tags = []string{"image_name:" + testImageName, "image_tag:v1"}
		m.resolvePendingEvents()

		assert.Equal(t, uint64(0), m.queueSize.Load())
		assert.Equal(t, uint64(0), m.pendingProfiles.Load())

		p := m.profiles[testSelector]
		require.NotNil(t, p)
		node := findTestEventNode(p, model.ExecEventType, "known")
		require.NotNil(t, node)
		require.NotEmpty(t, p.ActivityTree.ProcessNodes)
		var matchedRules []*model.MatchedRule
		for _, pn := range p.ActivityTree.ProcessNodes {
			if pn.Process.FileEvent.PathnameStr == "/usr/bin/known" {
				matchedRules = pn.MatchedRules
			}
		}
		require.Len(t, matchedRules, 1)
		assert.Equal(t, "rule_1", matchedRules[0].RuleID)
		require.Len(t, *anomalies, 1)
		require.Len(t, (*anomalies)[0].Rules, 1)
		assert.Equal(t, "rule_1", (*anomalies)[0].Rules[0].RuleID)
	})

	t.Run("drains pending events even if short-lived container cgroup already exited", func(t *testing.T) {
		m, anomalies := newInsertionTestManagerV2(t)
		tagger := &fakeContainerTagger{}
		m.resolvers.TagsResolver = tags.NewResolver(0, tagger, m.resolvers.CGroupResolver, nil)

		ev := newTestEventV2(model.ExecEventType, "v1", "known")
		ev.Timestamp = time.Time{}
		ev.FieldHandlers = &lazyTimeFieldHandlers{FakeFieldHandlers: &model.FakeFieldHandlers{}}
		ev.ProcessContext.Process.ContainerContext.Tags = nil

		m.queueEventForTagResolution(ev, nil)
		require.Equal(t, uint64(1), m.queueSize.Load())

		m.resolvers.CGroupResolver.Delete(4242)
		require.Nil(t, m.resolvers.CGroupResolver.GetCacheEntryContainerID(containerutils.ContainerID(testContainerID)))

		tagger.tags = []string{"image_name:" + testImageName, "image_tag:v1"}
		m.resolvePendingEvents()

		p := m.profiles[testSelector]
		require.NotNil(t, p)
		assert.NotNil(t, findTestEventNode(p, model.ExecEventType, "known"))
		assert.Empty(t, p.Instances)
		assert.Contains(t, m.pendingProfileRemovals, testSelector)
		assert.Empty(t, m.resolvedCgroups)
		require.Len(t, *anomalies, 1)
		assert.Equal(t, p.Metadata.Name, (*anomalies)[0].SecurityProfileContext.Name)
	})
}

func TestManagerV2_onCGroupDeleted_containerUnlink(t *testing.T) {
	m, _ := newInsertionTestManagerV2(t)

	_, inserted := m.insertEventIntoProfile(newTestEventV2(model.ExecEventType, "v1", "known"))
	require.True(t, inserted)

	p := m.profiles[testSelector]
	require.NotNil(t, p)
	require.Len(t, p.Instances, 1)

	cgce := m.resolvers.CGroupResolver.GetCacheEntryContainerID(containerutils.ContainerID(testContainerID))
	require.NotNil(t, cgce)

	m.onCGroupDeleted(cgce)
	assert.Empty(t, p.Instances)
	assert.Contains(t, m.pendingProfileRemovals, testSelector)
}

func TestManagerV2_ActivityDumpCLI(t *testing.T) {
	m, _ := newInsertionTestManagerV2(t)
	dir := t.TempDir()
	localStorage, err := storage.NewDirectory(dir, 100)
	require.NoError(t, err)
	m.localStorage = localStorage
	m.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout = 30 * time.Minute
	m.configuredStorageRequests = perFormatStorageRequests([]config.StorageRequest{
		config.NewStorageRequest(config.LocalStorage, config.Profile, false, dir),
		config.NewStorageRequest(config.LocalStorage, config.JSON, false, dir),
	})

	p, inserted := m.insertEventIntoProfile(newTestEventV2(model.ExecEventType, "v1", "known"))
	require.True(t, inserted)
	require.NotNil(t, p)

	listMsg, err := m.ListActivityDumps(&api.ActivityDumpListParams{})
	require.NoError(t, err)
	require.Len(t, listMsg.GetDumps(), 1)
	assert.Equal(t, p.Metadata.Name, listMsg.GetDumps()[0].GetMetadata().GetName())
	assert.Equal(t, testContainerID, listMsg.GetDumps()[0].GetMetadata().GetContainerID())

	stopMsg, err := m.StopActivityDump(&api.ActivityDumpStopParams{ContainerID: testContainerID})
	require.NoError(t, err)
	assert.Empty(t, stopMsg.GetError())

	profilePath := filepath.Join(dir, p.Metadata.Name+"."+config.Profile.String())
	jsonPath := filepath.Join(dir, p.Metadata.Name+"."+config.JSON.String())
	_, err = os.Stat(profilePath)
	require.NoError(t, err)
	_, err = os.Stat(jsonPath)
	require.NoError(t, err)

	transcodeDir := t.TempDir()
	transcodeMsg, err := m.GenerateTranscoding(&api.TranscodingRequestParams{
		ActivityDumpFile: profilePath,
		Storage: &api.StorageRequestParams{
			LocalStorageDirectory: transcodeDir,
			LocalStorageFormats:   []string{"json"},
		},
	})
	require.NoError(t, err)
	assert.Empty(t, transcodeMsg.GetError())
	require.Len(t, transcodeMsg.GetStorage(), 1)
	_, err = os.Stat(filepath.Join(transcodeDir, p.Metadata.Name+".json"))
	require.NoError(t, err)
}


