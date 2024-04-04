// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"sync"
	"time"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	timeResolver "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// EventTypeState defines an event type state
type EventTypeState struct {
	lastAnomalyNano uint64
	state           EventFilteringProfileState
}

// VersionContext holds the context of one version (defined by its image tag)
type VersionContext struct {
	firstSeenNano uint64
	lastSeenNano  uint64

	eventTypeState map[model.EventType]*EventTypeState

	// Syscalls is the syscalls profile
	Syscalls []uint32

	// Tags defines the tags used to compute this profile, for each present profile versions
	Tags []string
}

// SecurityProfile defines a security profile
type SecurityProfile struct {
	sync.Mutex
	timeResolver        *timeResolver.Resolver
	loadedInKernel      bool
	loadedNano          uint64
	selector            cgroupModel.WorkloadSelector
	profileCookie       uint64
	eventTypes          []model.EventType
	versionContextsLock sync.Mutex
	versionContexts     map[string]*VersionContext

	// Instances is the list of workload instances to witch the profile should apply
	Instances []*cgroupModel.CacheEntry

	// Metadata contains metadata for the current profile
	Metadata mtdt.Metadata

	// ActivityTree contains the activity tree of the Security Profile
	ActivityTree *activity_tree.ActivityTree
}

// NewSecurityProfile creates a new instance of Security Profile
func NewSecurityProfile(selector cgroupModel.WorkloadSelector, eventTypes []model.EventType) *SecurityProfile {
	// TODO: we need to keep track of which event types / fields can be used in profiles (for anomaly detection, hardening
	// or suppression). This is missing for now, and it will be necessary to smoothly handle the transition between
	// profiles that allow for evaluating new event types, and profiles that don't. As such, the event types allowed to
	// generate anomaly detections in the input of this function will need to be merged with the event types defined in
	// the configuration.
	tr, err := timeResolver.NewResolver()
	if err != nil {
		return nil
	}
	sp := &SecurityProfile{
		selector:        selector,
		eventTypes:      eventTypes,
		versionContexts: make(map[string]*VersionContext),
		timeResolver:    tr,
	}
	if selector.Tag != "" && selector.Tag != "*" {
		sp.versionContexts[selector.Tag] = &VersionContext{
			eventTypeState: make(map[model.EventType]*EventTypeState),
		}
	}
	return sp
}

// reset empties all internal fields so that this profile can be used again in the future
func (p *SecurityProfile) reset() {
	p.loadedInKernel = false
	p.loadedNano = 0
	p.profileCookie = 0
	p.versionContexts = make(map[string]*VersionContext)
	p.Instances = nil
}

// generateCookies computes random cookies for all the entries in the profile that require one
func (p *SecurityProfile) generateCookies() {
	p.profileCookie = utils.RandNonZeroUint64()

	// TODO: generate cookies for all the nodes in the activity tree
}

func (p *SecurityProfile) generateSyscallsFilters() [64]byte {
	var output [64]byte
	for _, pCtxt := range p.versionContexts {
		for _, syscall := range pCtxt.Syscalls {
			if syscall/8 < 64 && (1<<(syscall%8) < 256) {
				output[syscall/8] |= 1 << (syscall % 8)
			}
		}
	}
	return output
}

// MatchesSelector is used to control how an event should be added to a profile
func (p *SecurityProfile) MatchesSelector(entry *model.ProcessCacheEntry) bool {
	for _, workload := range p.Instances {
		if entry.ContainerID == workload.ID {
			return true
		}
	}
	return false
}

// IsEventTypeValid is used to control which event types should trigger anomaly detection alerts
func (p *SecurityProfile) IsEventTypeValid(evtType model.EventType) bool {
	return slices.Contains(p.eventTypes, evtType)
}

// NewProcessNodeCallback is a callback function used to propagate the fact that a new process node was added to the activity tree
func (p *SecurityProfile) NewProcessNodeCallback(_ *activity_tree.ProcessNode) {
	// TODO: debounce and regenerate profile filters & programs
}

// LoadProfileFromFile loads profile from file
func LoadProfileFromFile(filepath string) (*proto.SecurityProfile, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("couldn't open profile: %w", err)
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("couldn't open profile: %w", err)
	}

	profile := &proto.SecurityProfile{}
	if err = profile.UnmarshalVT(raw); err != nil {
		return nil, fmt.Errorf("couldn't decode protobuf profile: %w", err)
	}
	return profile, nil
}

// SendStats sends profile stats
func (p *SecurityProfile) SendStats(client statsd.ClientInterface) error {
	p.Lock()
	defer p.Unlock()
	return p.ActivityTree.SendStats(client)
}

// ToSecurityProfileMessage returns a SecurityProfileMessage filled with the content of the current Security Profile
func (p *SecurityProfile) ToSecurityProfileMessage() *api.SecurityProfileMessage {
	p.versionContextsLock.Lock()
	defer p.versionContextsLock.Unlock()

	// construct the list of image tags for this profile
	imageTags := ""
	for key := range p.versionContexts {
		if imageTags != "" {
			imageTags = imageTags + ","
		}
		imageTags = imageTags + key
	}

	msg := &api.SecurityProfileMessage{
		LoadedInKernel:          p.loadedInKernel,
		LoadedInKernelTimestamp: p.timeResolver.ResolveMonotonicTimestamp(p.loadedNano).String(),
		Selector: &api.WorkloadSelectorMessage{
			Name: p.selector.Image,
			Tag:  imageTags,
		},
		ProfileCookie: p.profileCookie,
		Metadata: &api.MetadataMessage{
			Name: p.Metadata.Name,
		},
		ProfileGlobalState: p.getGlobalState().String(),
		ProfileContexts:    make(map[string]*api.ProfileContextMessage),
	}
	for imageTag, ctx := range p.versionContexts {
		msgCtx := &api.ProfileContextMessage{
			FirstSeen:      ctx.firstSeenNano,
			LastSeen:       ctx.lastSeenNano,
			EventTypeState: make(map[string]*api.EventTypeState),
			Tags:           ctx.Tags,
		}
		for et, state := range ctx.eventTypeState {
			msgCtx.EventTypeState[et.String()] = &api.EventTypeState{
				LastAnomalyNano:   state.lastAnomalyNano,
				EventProfileState: state.state.String(),
			}
		}
		msg.ProfileContexts[imageTag] = msgCtx
	}

	if p.ActivityTree != nil {
		msg.Stats = &api.ActivityTreeStatsMessage{
			ProcessNodesCount: p.ActivityTree.Stats.ProcessNodes,
			FileNodesCount:    p.ActivityTree.Stats.FileNodes,
			DNSNodesCount:     p.ActivityTree.Stats.DNSNodes,
			SocketNodesCount:  p.ActivityTree.Stats.SocketNodes,
			ApproximateSize:   p.ActivityTree.Stats.ApproximateSize(),
		}
	}

	for _, evt := range p.eventTypes {
		msg.EventTypes = append(msg.EventTypes, evt.String())
	}

	for _, inst := range p.Instances {
		msg.Instances = append(msg.Instances, &api.InstanceMessage{
			ContainerID: inst.ID,
			Tags:        inst.Tags,
		})
	}
	return msg
}

// GetState returns the state of a profile for a given imageTag
func (p *SecurityProfile) GetState(imageTag string) EventFilteringProfileState {
	pCtx, ok := p.versionContexts[imageTag]
	if !ok {
		return NoProfile
	}
	if len(pCtx.eventTypeState) == 0 {
		return AutoLearning
	}
	state := StableEventType
	for _, et := range p.eventTypes {
		s, ok := pCtx.eventTypeState[et]
		if !ok {
			continue
		}
		if s.state == UnstableEventType {
			return UnstableEventType
		} else if s.state != StableEventType {
			state = AutoLearning
		}
	}
	return state
}

func (p *SecurityProfile) getGlobalState() EventFilteringProfileState {
	globalState := AutoLearning
	for imageTag := range p.versionContexts {
		state := p.GetState(imageTag)
		if state == UnstableEventType {
			return UnstableEventType
		} else if state == StableEventType {
			globalState = StableEventType
		}
	}
	return globalState // AutoLearning or StableEventType
}

// GetGlobalState returns the global state of a profile: AutoLearning, StableEventType or UnstableEventType
func (p *SecurityProfile) GetGlobalState() EventFilteringProfileState {
	p.versionContextsLock.Lock()
	defer p.versionContextsLock.Unlock()
	return p.getGlobalState()
}

// GetGlobalEventTypeState returns the global state of a profile for a given event type: AutoLearning, StableEventType or UnstableEventType
func (p *SecurityProfile) GetGlobalEventTypeState(et model.EventType) EventFilteringProfileState {
	globalState := AutoLearning
	for _, ctx := range p.versionContexts {
		s, ok := ctx.eventTypeState[et]
		if !ok {
			continue
		}
		if s.state == UnstableEventType {
			return UnstableEventType
		} else if s.state == StableEventType {
			globalState = StableEventType
		}
	}
	return globalState // AutoLearning or StableEventType
}

func (p *SecurityProfile) evictProfileVersion() string {
	if len(p.versionContexts) <= 0 {
		return "" // should not happen
	}

	oldest := uint64(math.MaxUint64)
	oldestImageTag := ""

	// select the oldest image tag
	// TODO: not 100% sure to select the first or the lastSeenNano
	for imageTag, profileCtx := range p.versionContexts {
		if profileCtx.lastSeenNano < oldest {
			oldest = profileCtx.lastSeenNano
			oldestImageTag = imageTag
		}
	}
	// delete image context
	delete(p.versionContexts, oldestImageTag)

	// then, remove every trace of this version from the tree
	p.ActivityTree.EvictImageTag(oldestImageTag)
	return oldestImageTag
}

func (p *SecurityProfile) makeRoomForNewVersion(maxImageTags int) []string {
	evictedVersions := []string{}
	// if we reached the max number of versions, we should evict the surplus
	surplus := len(p.versionContexts) - maxImageTags + 1
	for surplus > 0 {
		evictedVersion := p.evictProfileVersion()
		if evictedVersion != "" {
			evictedVersions = append(evictedVersions, evictedVersion)
		}
		surplus--
	}
	return evictedVersions
}

func (p *SecurityProfile) prepareNewVersion(newImageTag string, tags []string, maxImageTags int) []string {
	// prepare new profile context to be inserted
	now := uint64(p.timeResolver.ComputeMonotonicTimestamp(time.Now()))
	newProfileCtx := &VersionContext{
		eventTypeState: make(map[model.EventType]*EventTypeState),
		firstSeenNano:  now,
		lastSeenNano:   now,
		Tags:           tags,
	}

	// add the new profile context to the list
	// (versionContextsLock already locked here)
	evictedVersions := p.makeRoomForNewVersion(maxImageTags)
	p.versionContexts[newImageTag] = newProfileCtx
	return evictedVersions
}

func (p *SecurityProfile) getTimeOrderedVersionContexts() []*VersionContext {
	var orderedVersions []*VersionContext

	for _, version := range p.versionContexts {
		orderedVersions = append(orderedVersions, version)
	}
	slices.SortFunc(orderedVersions, func(a, b *VersionContext) int {
		if a.firstSeenNano == b.firstSeenNano {
			return 0
		} else if a.firstSeenNano < b.firstSeenNano {
			return -1
		}
		return 1
	})
	return orderedVersions
}

// GetVersionContextIndex returns the context of the givent version if any
func (p *SecurityProfile) GetVersionContextIndex(index int) *VersionContext {
	p.versionContextsLock.Lock()
	orderedVersions := p.getTimeOrderedVersionContexts()
	p.versionContextsLock.Unlock()

	if index >= len(orderedVersions) {
		return nil
	}
	return orderedVersions[index]
}

// ListAllVersionStates is a debug function to list all version and their states
func (p *SecurityProfile) ListAllVersionStates() {
	p.versionContextsLock.Lock()
	orderedVersions := p.getTimeOrderedVersionContexts()
	p.versionContextsLock.Unlock()

	versions := ""
	for version := range p.versionContexts {
		versions += version + " "
	}
	fmt.Printf("Versions: %s\n", versions)

	fmt.Printf("Global state: %s\n", p.GetGlobalState().String())
	for i, version := range orderedVersions {
		fmt.Printf("Version %d:\n", i)
		fmt.Printf("  - Tags: %+v\n", version.Tags)
		fmt.Printf("  - First seen: %v\n", time.Unix(0, int64(version.firstSeenNano)))
		fmt.Printf("  - Last seen: %v\n", time.Unix(0, int64(version.lastSeenNano)))
		fmt.Printf("  - Event types:\n")
		for et, ets := range version.eventTypeState {
			fmt.Printf("    . %s: %+v\n", et, ets.state.String())
		}
	}
	fmt.Printf("Instances:\n")
	for _, instance := range p.Instances {
		fmt.Printf("  - %+v\n", instance.ID)
	}
}

// SetVersionState force a state for a given version (debug purpose only)
func (p *SecurityProfile) SetVersionState(imageTag string, state EventFilteringProfileState) error {
	p.versionContextsLock.Lock()
	defer p.versionContextsLock.Unlock()

	ctx, found := p.versionContexts[imageTag]
	if !found {
		return errors.New("profile version not found")
	}
	now := uint64(p.timeResolver.ComputeMonotonicTimestamp(time.Now()))
	for _, et := range p.eventTypes {
		if eventState, ok := ctx.eventTypeState[et]; ok {
			eventState.state = state
		} else {
			ctx.eventTypeState[et] = &EventTypeState{
				lastAnomalyNano: now,
				state:           state,
			}
		}
	}
	return nil
}

// GetVersions returns the number of versions stored in the profile (debug purpose only)
func (p *SecurityProfile) GetVersions() []string {
	p.versionContextsLock.Lock()
	defer p.versionContextsLock.Unlock()
	versions := []string{}
	for version := range p.versionContexts {
		versions = append(versions, version)
	}
	return versions
}
