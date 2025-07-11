// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"

	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
)

// VersionContext holds the context of one version (defined by its image tag)
type VersionContext struct {
	FirstSeenNano uint64
	LastSeenNano  uint64

	EventTypeState map[model.EventType]*EventTypeState

	// Syscalls is the syscalls profile
	Syscalls []uint32

	// Tags defines the tags used to compute this profile, for each present profile versions
	Tags []string
}

// EventTypeState defines an event type state
type EventTypeState struct {
	LastAnomalyNano uint64
	State           model.EventFilteringProfileState
}

type activityTreeOpts struct {
	pathsReducer      *activity_tree.PathsReducer
	differentiateArgs bool
	dnsMatchMaxDepth  int
}

// Profile represents a security profile
type Profile struct {
	// common to ActivityDump and SecurityProfile
	m            sync.Mutex
	ActivityTree *activity_tree.ActivityTree
	treeOpts     activityTreeOpts

	Header   ActivityDumpHeader
	Metadata mtdt.Metadata
	selector cgroupModel.WorkloadSelector
	tags     []string

	versionContexts map[string]*VersionContext

	eventTypes []model.EventType

	// fields from SecurityProfile
	// TODO: move the following fields to a dedicated "WorkloadProfileGroup" struct responsible for managing the profile instances
	profileCookie  uint64
	LoadedInKernel *atomic.Bool
	LoadedNano     *atomic.Uint64
	// Instances is the list of workload instances to witch the profile should apply
	InstancesLock sync.Mutex
	Instances     []*tags.Workload
}

// Opts defines the options to create a new profile
type Opts func(*Profile)

// WithWorkloadSelector sets the workload selector of a new profile
func WithWorkloadSelector(selector cgroupModel.WorkloadSelector) Opts {
	return func(p *Profile) {
		p.selector = selector
	}
}

// WithEventTypes sets the event types of a new profile
func WithEventTypes(eventTypes []model.EventType) Opts {
	return func(p *Profile) {
		p.eventTypes = eventTypes
	}
}

// WithPathsReducer sets the path reducer of a new profile
func WithPathsReducer(pathsReducer *activity_tree.PathsReducer) Opts {
	return func(p *Profile) {
		p.treeOpts.pathsReducer = pathsReducer
	}
}

// WithDifferentiateArgs sets whether arguments should be used to differentiate processes in the new profile
func WithDifferentiateArgs(differentiateArgs bool) Opts {
	return func(p *Profile) {
		p.treeOpts.differentiateArgs = differentiateArgs
	}
}

// WithDNSMatchMaxDepth sets the maximum depth used to compare domain names in the new profile
func WithDNSMatchMaxDepth(dnsMatchMaxDepth int) Opts {
	return func(p *Profile) {
		p.treeOpts.dnsMatchMaxDepth = dnsMatchMaxDepth
	}
}

// New returns a new profile
func New(opts ...Opts) *Profile {
	p := &Profile{
		Header: ActivityDumpHeader{
			DNSNames: utils.NewStringKeys(nil),
		},
		LoadedInKernel:  atomic.NewBool(false),
		LoadedNano:      atomic.NewUint64(0),
		versionContexts: make(map[string]*VersionContext),
		profileCookie:   utils.RandNonZeroUint64(),
	}

	for _, opt := range opts {
		opt(p)
	}

	p.ActivityTree = activity_tree.NewActivityTree(p, p.treeOpts.pathsReducer, "security_profile")
	p.ActivityTree.DNSMatchMaxDepth = p.treeOpts.dnsMatchMaxDepth
	if p.treeOpts.differentiateArgs {
		p.ActivityTree.DifferentiateArgs()
	}

	if p.selector.Tag != "" && p.selector.Tag != "*" {
		p.versionContexts[p.selector.Tag] = &VersionContext{
			EventTypeState: make(map[model.EventType]*EventTypeState),
		}
	}

	return p
}

// SetTreeType updates the type and owner of the ActivityTree of this profile
func (p *Profile) SetTreeType(validator activity_tree.Owner, treeType string) {
	p.m.Lock()
	defer p.m.Unlock()
	p.ActivityTree.SetType(treeType, validator)
}

// GetSelectorStr returns the string representation of the profile selector
func (p *Profile) GetSelectorStr() string {
	p.m.Lock()
	defer p.m.Unlock()
	return p.getSelectorStr()
}

// getSelectorStr internal, thread-unsafe version of GetSelectorStr
func (p *Profile) getSelectorStr() string {
	if p.selector.IsReady() {
		return p.selector.String()
	}
	return "empty_selector"
}

// Encode encodes an activity dump in the provided format
func (p *Profile) Encode(format config.StorageFormat) (*bytes.Buffer, error) {
	switch format {
	case config.JSON:
		return p.EncodeJSON("")
	case config.Protobuf:
		return p.EncodeSecDumpProtobuf()
	case config.Dot:
		return p.EncodeDOT()
	case config.Profile:
		return p.EncodeSecurityProfileProtobuf()
	default:
		return nil, fmt.Errorf("couldn't encode activity dump [%s] as [%s]: unknown format", p.GetSelectorStr(), format)
	}
}

// Decode decodes an activity dump from a file
func (p *Profile) Decode(inputFile string) error {
	var err error
	ext := filepath.Ext(inputFile)

	if ext == ".gz" {
		inputFile, err = unzip(inputFile, ext)
		if err != nil {
			return err
		}
		ext = filepath.Ext(inputFile)
	}

	format, err := config.ParseStorageFormat(ext)
	if err != nil {
		return err
	}

	f, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("couldn't open activity dump file: %w", err)
	}
	defer f.Close()

	return p.DecodeFromReader(f, format)
}

// DecodeFromReader decodes an activity dump from a reader with the provided format
func (p *Profile) DecodeFromReader(reader io.Reader, format config.StorageFormat) error {
	switch format {
	case config.Protobuf:
		return p.DecodeSecDumpProtobuf(reader)
	case config.Profile:
		return p.DecodeSecurityProfileProtobuf(reader)
	case config.JSON:
		return p.DecodeJSON(reader)
	default:
		return fmt.Errorf("unsupported input format: %s", format)
	}
}

// IsEmpty return true if the dump did not contain any nodes
func (p *Profile) IsEmpty() bool {
	p.m.Lock()
	defer p.m.Unlock()
	return p.ActivityTree.IsEmpty()
}

// InsertAndGetSize inserts an event in the profile and returns the new size of the profile if the event was inserted
func (p *Profile) InsertAndGetSize(event *model.Event, insertMissingProcesses bool, imageTag string, generationType activity_tree.NodeGenerationType, resolvers *resolvers.EBPFResolvers) (bool, int64, error) {
	p.m.Lock()
	defer p.m.Unlock()

	ok, err := p.ActivityTree.Insert(event, insertMissingProcesses, imageTag, generationType, resolvers)
	if !ok || err != nil {
		return ok, 0, err
	}

	return ok, p.ActivityTree.Stats.ApproximateSize(), nil
}

// Insert inserts an event in the profile
func (p *Profile) Insert(event *model.Event, insertMissingProcesses bool, imageTag string, generationType activity_tree.NodeGenerationType, resolvers *resolvers.EBPFResolvers) (bool, error) {
	p.m.Lock()
	defer p.m.Unlock()

	return p.ActivityTree.Insert(event, insertMissingProcesses, imageTag, generationType, resolvers)
}

// ComputeInMemorySize returns the size of a dump in memory
func (p *Profile) ComputeInMemorySize() int64 {
	p.m.Lock()
	defer p.m.Unlock()
	return p.ActivityTree.Stats.ApproximateSize()
}

// FakeOverweight fakes an overweight profile
func (p *Profile) FakeOverweight() {
	p.m.Lock()
	defer p.m.Unlock()
	p.ActivityTree.Stats.ProcessNodes = 99999
}

// AddTags adds tags to the profile
func (p *Profile) AddTags(tags []string) {
	p.m.Lock()
	defer p.m.Unlock()

	existingTagNames := make([]string, 0, len(p.tags))
	for _, tag := range p.tags {
		existingTagNames = append(existingTagNames, utils.GetTagName(tag))
	}

	for _, tag := range tags {
		tagName := utils.GetTagName(tag)
		if !slices.Contains(existingTagNames, tagName) {
			p.tags = append(p.tags, tag)
		}
	}
}

// GetTagValue returns the value of the given tag name
func (p *Profile) GetTagValue(tagName string) string {
	p.m.Lock()
	defer p.m.Unlock()

	return utils.GetTagValue(tagName, p.tags)
}

// HasTag returns true if the profile has the given tag
func (p *Profile) HasTag(tag string) bool {
	p.m.Lock()
	defer p.m.Unlock()

	return slices.Contains(p.tags, tag)
}

// GetTags returns a copy of the profile tags
func (p *Profile) GetTags() []string {
	p.m.Lock()
	defer p.m.Unlock()

	tags := make([]string, len(p.tags))
	copy(tags, p.tags)
	return tags
}

// ScrubProcessArgsEnvs scrubs the process arguments and environment variables
func (p *Profile) ScrubProcessArgsEnvs(resolver *process.EBPFResolver) {
	p.m.Lock()
	defer p.m.Unlock()

	p.ActivityTree.ScrubProcessArgsEnvs(resolver)
}

// Snapshot collects procfs data for all the processes in the activity tree
func (p *Profile) Snapshot(newEvent func() *model.Event) {
	p.m.Lock()
	defer p.m.Unlock()

	p.ActivityTree.Snapshot(newEvent)
}

// GetWorkloadSelector returns the workload selector
func (p *Profile) GetWorkloadSelector() *cgroupModel.WorkloadSelector {
	p.m.Lock()
	defer p.m.Unlock()

	if p.selector.IsReady() {
		return &p.selector
	}

	var selector cgroupModel.WorkloadSelector
	var err error
	if p.Metadata.ContainerID != "" {
		imageTag := utils.GetTagValue("image_tag", p.tags)
		selector, err = cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", p.tags), imageTag)
		if err != nil {
			return nil
		}
	} else if p.Metadata.CGroupContext.CGroupID != "" {
		selector, err = cgroupModel.NewWorkloadSelector(utils.GetTagValue("service", p.tags), utils.GetTagValue("version", p.tags))
		if err != nil {
			return nil
		}
	}

	p.selector = selector
	// Once per workload, when tags are resolved and the first time we successfully get the selector, tag all the existing nodes
	p.ActivityTree.TagAllNodes(selector.Tag, time.Now())

	return &p.selector
}

// SendStats sends stats for this profile's activity tree
func (p *Profile) SendStats(statsdClient statsd.ClientInterface) error {
	p.m.Lock()
	defer p.m.Unlock()

	return p.ActivityTree.SendStats(statsdClient)
}

// AddSnapshotAncestors adds the given process branch to the profile, calling the callback for each process cache entry which resulted in a new node insertion
func (p *Profile) AddSnapshotAncestors(ancestors []*model.ProcessCacheEntry, resolvers *resolvers.EBPFResolvers, callback func(*model.ProcessCacheEntry)) {
	p.m.Lock()
	defer p.m.Unlock()

	imageTag := utils.GetTagValue("image_tag", p.tags)

	for _, process := range ancestors {
		node, _, err := p.ActivityTree.CreateProcessNode(process, imageTag, activity_tree.Snapshot, false, resolvers)
		if err != nil {
			continue
		}
		if node != nil && callback != nil {
			callback(process)
		}
	}
}

// Contains checks if the profile contains the given event
func (p *Profile) Contains(event *model.Event, insertMissingProcesses bool, imageTag string, generationType activity_tree.NodeGenerationType, resolvers *resolvers.EBPFResolvers) (bool, error) {
	p.m.Lock()
	defer p.m.Unlock()

	return p.ActivityTree.Contains(event, insertMissingProcesses, imageTag, generationType, resolvers)
}

// SecurityProfile funcs

// GetProfileCookie returns the profile cookie
func (p *Profile) GetProfileCookie() uint64 {
	p.m.Lock()
	defer p.m.Unlock()

	return p.profileCookie
}

// GenerateSyscallsFilters generates the syscall filters for the profile
func (p *Profile) GenerateSyscallsFilters() [64]byte {
	p.m.Lock()
	defer p.m.Unlock()

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

// GetState returns the state of a profile for a given imageTag
func (p *Profile) getState(imageTag string) model.EventFilteringProfileState {
	pCtx, ok := p.versionContexts[imageTag]
	if !ok {
		return model.NoProfile
	}
	if len(pCtx.EventTypeState) == 0 {
		return model.AutoLearning
	}
	state := model.StableEventType
	for _, et := range p.eventTypes {
		s, ok := pCtx.EventTypeState[et]
		if !ok {
			continue
		}
		if s.State == model.UnstableEventType {
			return model.UnstableEventType
		} else if s.State != model.StableEventType {
			state = model.AutoLearning
		}
	}
	return state
}

func (p *Profile) getGlobalState() model.EventFilteringProfileState {
	globalState := model.AutoLearning
	for imageTag := range p.versionContexts {
		state := p.getState(imageTag)
		if state == model.UnstableEventType {
			return model.UnstableEventType
		} else if state == model.StableEventType {
			globalState = model.StableEventType
		}
	}
	return globalState // AutoLearning or StableEventType
}

// GetVersionContext returns the context of the givent version if any
func (p *Profile) GetVersionContext(imageTag string) (*VersionContext, bool) {
	p.m.Lock()
	defer p.m.Unlock()

	ctx, ok := p.versionContexts[imageTag]
	return ctx, ok
}

// GetVersions returns the number of versions stored in the profile (debug purpose only)
func (p *Profile) GetVersions() []string {
	p.m.Lock()
	defer p.m.Unlock()
	versions := []string{}
	for version := range p.versionContexts {
		versions = append(versions, version)
	}
	return versions
}

// SetVersionState force a state for a given version (debug purpose only)
func (p *Profile) SetVersionState(imageTag string, state model.EventFilteringProfileState, lastAnomalyNano uint64) error {
	p.m.Lock()
	defer p.m.Unlock()

	ctx, found := p.versionContexts[imageTag]
	if !found {
		return errors.New("profile version not found")
	}
	for _, et := range p.eventTypes {
		if eventState, ok := ctx.EventTypeState[et]; ok {
			eventState.State = state
		} else {
			ctx.EventTypeState[et] = &EventTypeState{
				LastAnomalyNano: lastAnomalyNano,
				State:           state,
			}
		}
	}
	return nil
}

// IsEventTypeValid returns true if the event type is valid for the profile
func (p *Profile) IsEventTypeValid(evtType model.EventType) bool {
	return slices.Contains(p.eventTypes, evtType)
}

// GetGlobalEventTypeState returns the global state of a profile for a given event type: AutoLearning, StableEventType or UnstableEventType
func (p *Profile) GetGlobalEventTypeState(et model.EventType) model.EventFilteringProfileState {
	p.m.Lock()
	defer p.m.Unlock()

	globalState := model.AutoLearning
	for _, ctx := range p.versionContexts {
		s, ok := ctx.EventTypeState[et]
		if !ok {
			continue
		}
		if s.State == model.UnstableEventType {
			return model.UnstableEventType
		} else if s.State == model.StableEventType {
			globalState = model.StableEventType
		}
	}
	return globalState // AutoLearning or StableEventType
}

// PrepareNewVersion prepares a new version of the profile
func (p *Profile) PrepareNewVersion(newImageTag string, tags []string, maxImageTags int, nowTimestamp uint64) []string {
	p.m.Lock()
	defer p.m.Unlock()

	// prepare new profile context to be inserted
	newProfileCtx := &VersionContext{
		EventTypeState: make(map[model.EventType]*EventTypeState),
		FirstSeenNano:  nowTimestamp,
		LastSeenNano:   nowTimestamp,
		Tags:           tags,
	}

	// add the new profile context to the list
	evictedVersions := p.makeRoomForNewVersion(maxImageTags)
	p.versionContexts[newImageTag] = newProfileCtx
	return evictedVersions
}

// AddVersionContext adds a new version context to the profile
func (p *Profile) AddVersionContext(version string, ctx *VersionContext) {
	p.m.Lock()
	defer p.m.Unlock()

	p.versionContexts[version] = ctx
}

func (p *Profile) makeRoomForNewVersion(maxImageTags int) []string {
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

func (p *Profile) evictProfileVersion() string {
	if len(p.versionContexts) <= 0 {
		return "" // should not happen
	}

	oldest := uint64(math.MaxUint64)
	oldestImageTag := ""

	// select the oldest image tag
	// TODO: not 100% sure to select the first or the lastSeenNano
	for imageTag, profileCtx := range p.versionContexts {
		if profileCtx.LastSeenNano < oldest {
			oldest = profileCtx.LastSeenNano
			oldestImageTag = imageTag
		}
	}
	// delete image context
	delete(p.versionContexts, oldestImageTag)

	// then, remove every trace of this version from the tree
	p.ActivityTree.EvictImageTag(oldestImageTag)
	return oldestImageTag
}

// GetEventTypes returns the event types of the profile
func (p *Profile) GetEventTypes() []model.EventType {
	return p.eventTypes
}

// LoadFromNewProfile loads a new profile into the current profile
func (p *Profile) LoadFromNewProfile(newProfile *Profile) {
	p.m.Lock()
	defer p.m.Unlock()

	p.Metadata = newProfile.Metadata
	p.selector = newProfile.selector
	p.ActivityTree = newProfile.ActivityTree
	p.ActivityTree.SetType("security_profile", p)
	p.Header = newProfile.Header
	p.tags = newProfile.tags
	p.versionContexts = newProfile.versionContexts

	p.LoadedInKernel.Store(false)
	p.ActivityTree.ComputeActivityTreeStats()

	// if the input is an activity dump then change the selector to a profile selector
	if newProfile.selector.Tag != "*" {
		p.selector.Tag = "*"
	}
}

// Reset empties all internal fields so that this profile can be used again in the future
func (p *Profile) Reset() {
	p.LoadedInKernel.Store(false)
	p.LoadedNano.Store(0)
	p.Instances = nil
	// keep the profileCookie in case we end up reloading the profile from the cache
}

// ComputeSyscallsList computes the top level list of syscalls
func (p *Profile) ComputeSyscallsList() []uint32 {
	p.m.Lock()
	defer p.m.Unlock()

	return p.ActivityTree.ComputeSyscallsList()
}

// MatchesSelector is used to control how an event should be added to a profile
func (p *Profile) MatchesSelector(entry *model.ProcessCacheEntry) bool {
	for _, workload := range p.Instances {
		if entry.ContainerID == workload.ContainerID {
			return true
		}
	}
	return false
}

// NewProcessNodeCallback is called when a new process node is created
func (p *Profile) NewProcessNodeCallback(_ *activity_tree.ProcessNode) {}

// GetImageNameTag returns the image name and tag for the profiled container
func (p *Profile) GetImageNameTag() (string, string) {
	p.m.Lock()
	defer p.m.Unlock()

	return p.selector.Image, p.selector.Tag
}

func (p *Profile) getTimeOrderedVersionContexts() []*VersionContext {
	var orderedVersions []*VersionContext

	for _, version := range p.versionContexts {
		orderedVersions = append(orderedVersions, version)
	}
	slices.SortFunc(orderedVersions, func(a, b *VersionContext) int {
		if a.FirstSeenNano == b.FirstSeenNano {
			return 0
		} else if a.FirstSeenNano < b.FirstSeenNano {
			return -1
		}
		return 1
	})
	return orderedVersions
}

// GetVersionContextIndex returns the context of the givent version if any
func (p *Profile) GetVersionContextIndex(index int) *VersionContext {
	p.m.Lock()
	orderedVersions := p.getTimeOrderedVersionContexts()
	p.m.Unlock()

	if index >= len(orderedVersions) {
		return nil
	}
	return orderedVersions[index]
}

// ListAllVersionStates prints the state of all versions of the profile
func (p *Profile) ListAllVersionStates() {
	p.m.Lock()
	defer p.m.Unlock()

	if len(p.versionContexts) > 0 {
		fmt.Printf("### Profile: %+v\n", p.GetSelectorStr())
		orderedVersions := p.getTimeOrderedVersionContexts()

		versions := ""
		for version := range p.versionContexts {
			versions += version + " "
		}
		fmt.Printf("Versions: %s\n", versions)

		fmt.Printf("Global state: %s\n", p.getGlobalState().String())
		for i, version := range orderedVersions {
			fmt.Printf("Version %d:\n", i)
			fmt.Printf("  - Tags: %+v\n", version.Tags)
			fmt.Printf("  - First seen: %v\n", time.Unix(0, int64(version.FirstSeenNano)))
			fmt.Printf("  - Last seen: %v\n", time.Unix(0, int64(version.LastSeenNano)))
			fmt.Printf("  - Event types:\n")
			for et, ets := range version.EventTypeState {
				fmt.Printf("    . %s: %+v\n", et, ets.State.String())
			}
		}
		fmt.Printf("Instances:\n")
		for _, instance := range p.Instances {
			fmt.Printf("  - %+v\n", instance.ContainerID)
		}

	}
}
