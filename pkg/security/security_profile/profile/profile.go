// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/exp/slices"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
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

// SecurityProfile defines a security profile
type SecurityProfile struct {
	sync.Mutex
	loadedInKernel         bool
	loadedNano             uint64
	selector               cgroupModel.WorkloadSelector
	profileCookie          uint64
	anomalyDetectionEvents []model.EventType
	eventTypeState         map[model.EventType]*EventTypeState
	eventTypeStateLock     sync.Mutex

	// Instances is the list of workload instances to witch the profile should apply
	Instances []*cgroupModel.CacheEntry

	// Status is the status of the profile
	Status model.Status

	// Version is the version of a Security Profile
	Version string

	// Metadata contains metadata for the current profile
	Metadata mtdt.Metadata

	// Tags defines the tags used to compute this profile
	Tags []string

	// Syscalls is the syscalls profile
	Syscalls []uint32

	// ActivityTree contains the activity tree of the Security Profile
	ActivityTree *activity_tree.ActivityTree
}

// NewSecurityProfile creates a new instance of Security Profile
func NewSecurityProfile(selector cgroupModel.WorkloadSelector, anomalyDetectionEvents []model.EventType) *SecurityProfile {
	// TODO: we need to keep track of which event types / fields can be used in profiles (for anomaly detection, hardening
	// or suppression). This is missing for now, and it will be necessary to smoothly handle the transition between
	// profiles that allow for evaluating new event types, and profiles that don't. As such, the event types allowed to
	// generate anomaly detections in the input of this function will need to be merged with the event types defined in
	// the configuration.
	return &SecurityProfile{
		selector:               selector,
		anomalyDetectionEvents: anomalyDetectionEvents,
		eventTypeState:         make(map[model.EventType]*EventTypeState),
	}
}

// reset empties all internal fields so that this profile can be used again in the future
func (p *SecurityProfile) reset() {
	p.loadedInKernel = false
	p.loadedNano = 0
	p.profileCookie = 0
	p.eventTypeState = make(map[model.EventType]*EventTypeState)
	p.Instances = nil
}

// generateCookies computes random cookies for all the entries in the profile that require one
func (p *SecurityProfile) generateCookies() {
	p.profileCookie = utils.RandNonZeroUint64()

	// TODO: generate cookies for all the nodes in the activity tree
}

func (p *SecurityProfile) generateSyscallsFilters() [64]byte {
	var output [64]byte
	for _, syscall := range p.Syscalls {
		if syscall/8 < 64 && (1<<(syscall%8) < 256) {
			output[syscall/8] |= 1 << (syscall % 8)
		}
	}
	return output
}

func (p *SecurityProfile) generateKernelSecurityProfileDefinition() [16]byte {
	var output [16]byte
	model.ByteOrder.PutUint64(output[0:8], p.profileCookie)
	model.ByteOrder.PutUint32(output[8:12], uint32(p.Status))
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
	return slices.Contains(p.anomalyDetectionEvents, evtType)
}

// NewProcessNodeCallback is a callback function used to propagate the fact that a new process node was added to the activity tree
func (p *SecurityProfile) NewProcessNodeCallback(node *activity_tree.ProcessNode) {
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

	if len(utils.GetTagValue("image_tag", profile.Tags)) == 0 {
		profile.Tags = append(profile.Tags, "image_tag:latest")
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
func (p *SecurityProfile) ToSecurityProfileMessage(timeResolver *timeResolver.Resolver, cfg *config.RuntimeSecurityConfig) *api.SecurityProfileMessage {
	msg := &api.SecurityProfileMessage{
		LoadedInKernel:          p.loadedInKernel,
		LoadedInKernelTimestamp: timeResolver.ResolveMonotonicTimestamp(p.loadedNano).String(),
		Selector: &api.WorkloadSelectorMessage{
			Name: p.selector.Image,
			Tag:  p.selector.Tag,
		},
		ProfileCookie: p.profileCookie,
		Status:        p.Status.String(),
		Version:       p.Version,
		Metadata: &api.MetadataMessage{
			Name: p.Metadata.Name,
		},
		Tags: p.Tags,
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

	for _, evt := range p.anomalyDetectionEvents {
		msg.AnomalyDetectionEvents = append(msg.AnomalyDetectionEvents, evt.String())
	}

	for evt, state := range p.eventTypeState {
		lastAnomaly := timeResolver.ResolveMonotonicTimestamp(state.lastAnomalyNano)
		msg.LastAnomalies = append(msg.LastAnomalies, &api.LastAnomalyTimestampMessage{
			EventType:         evt.String(),
			Timestamp:         lastAnomaly.String(),
			IsStableEventType: time.Since(lastAnomaly) >= cfg.GetAnomalyDetectionMinimumStablePeriod(evt),
		})
	}

	for _, inst := range p.Instances {
		msg.Instances = append(msg.Instances, &api.InstanceMessage{
			ContainerID: inst.ID,
			Tags:        inst.Tags,
		})
	}
	return msg
}
