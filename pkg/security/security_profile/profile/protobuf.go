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
	"os"
	"slices"

	adprotov1 "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
)

// profileToSecDumpProto creates a protobuf SecDump object from the given Profile
func profileToSecDumpProto(p *Profile) *adprotov1.SecDump {
	if p == nil {
		return nil
	}

	pad := adprotov1.SecDumpFromVTPool()
	*pad = adprotov1.SecDump{
		Host:     p.Header.Host,
		Service:  p.Header.Service,
		Source:   p.Header.Source,
		Metadata: mtdt.ToProto(&p.Metadata),
		Tags:     make([]string, len(p.tags)),
		Tree:     activity_tree.ToProto(p.ActivityTree),
	}
	copy(pad.Tags, p.tags)

	return pad
}

// secDumpProtoToProfile converts a SecDump protobuf object to a Profile
func secDumpProtoToProfile(p *Profile, ad *adprotov1.SecDump) {
	if ad == nil {
		return
	}

	p.Header.Host = ad.Host
	p.Header.Service = ad.Service
	p.Header.Source = ad.Source
	p.Metadata = mtdt.ProtoMetadataToMetadata(ad.Metadata)

	p.tags = make([]string, len(ad.Tags))
	copy(p.tags, ad.Tags)

	activity_tree.ProtoDecodeActivityTree(p.ActivityTree, ad.Tree)
}

// EncodeSecDumpProtobuf encodes a Profile to its SecDump protobuf binary representation
func (p *Profile) EncodeSecDumpProtobuf() (*bytes.Buffer, error) {
	p.Lock()
	defer p.Unlock()

	pad := profileToSecDumpProto(p)
	defer pad.ReturnToVTPool()

	raw, err := pad.MarshalVT()
	if err != nil {
		return nil, fmt.Errorf("couldn't encode in protobuf: %v", err)
	}
	return bytes.NewBuffer(raw), nil
}

// DecodeSecDumpProtobuf decodes a SecDump binary representation
func (p *Profile) DecodeSecDumpProtobuf(reader io.Reader) error {
	p.Lock()
	defer p.Unlock()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("couldn't open activity dump file: %w", err)
	}

	inter := &adprotov1.SecDump{}
	if err := inter.UnmarshalVT(raw); err != nil {
		return fmt.Errorf("couldn't decode protobuf activity dump file: %w", err)
	}

	secDumpProtoToProfile(p, inter)

	return nil
}

// Security Profiles protobuf encoding/decoding

// eventFilteringProfileStateToProto convert a profile state to a proto one
func eventFilteringProfileStateToProto(efr model.EventFilteringProfileState) adprotov1.EventProfileState {
	switch efr {
	case model.NoProfile:
		return adprotov1.EventProfileState_NO_PROFILE
	case model.ProfileAtMaxSize:
		return adprotov1.EventProfileState_PROFILE_AT_MAX_SIZE
	case model.UnstableEventType:
		return adprotov1.EventProfileState_UNSTABLE_PROFILE
	case model.StableEventType:
		return adprotov1.EventProfileState_STABLE_PROFILE
	case model.AutoLearning:
		return adprotov1.EventProfileState_AUTO_LEARNING
	case model.WorkloadWarmup:
		return adprotov1.EventProfileState_WORKLOAD_WARMUP
	}
	return adprotov1.EventProfileState_NO_PROFILE
}

// protoToState converts a proto state to a profile one
func protoToState(eps adprotov1.EventProfileState) model.EventFilteringProfileState {
	switch eps {
	case adprotov1.EventProfileState_NO_PROFILE:
		return model.NoProfile
	case adprotov1.EventProfileState_PROFILE_AT_MAX_SIZE:
		return model.ProfileAtMaxSize
	case adprotov1.EventProfileState_UNSTABLE_PROFILE:
		return model.UnstableEventType
	case adprotov1.EventProfileState_STABLE_PROFILE:
		return model.StableEventType
	case adprotov1.EventProfileState_AUTO_LEARNING:
		return model.AutoLearning
	case adprotov1.EventProfileState_WORKLOAD_WARMUP:
		return model.WorkloadWarmup
	}
	return model.NoProfile
}

// profileToSecurityProfileProto creates a protobuf SecurityProfile object from the given Profile
func profileToSecurityProfileProto(p *Profile) (*adprotov1.SecurityProfile, error) {
	if p == nil {
		return nil, errors.New("imput == nil")
	}

	if !p.selector.IsReady() {
		return nil, errors.New("can't get profile selector, tags shouldn't be resolved yet")
	}

	output := adprotov1.SecurityProfile{
		Metadata:        mtdt.ToProto(&p.Metadata),
		ProfileContexts: make(map[string]*adprotov1.ProfileContext),
		Tree:            activity_tree.ToProto(p.ActivityTree),
		Selector:        cgroupModel.WorkloadSelectorToProto(&p.selector),
		Disabled:        !p.isEnabled,
	}

	var syscallsByImageTagID map[uint64][]uint32
	var capabilitiesByImageTagID map[uint64]*activity_tree.CapabilityRollup
	if p.observedRollups {
		syscallsByImageTagID = p.ActivityTree.SyscallsByImageTagID()
		capabilitiesByImageTagID = p.ActivityTree.CapabilitiesByImageTagID()
	}

	for key, ctx := range p.versionContexts {
		syscalls := ctx.Syscalls
		var capabilities *activity_tree.CapabilityRollup
		if p.observedRollups {
			imageTagID := p.ActivityTree.GetImageTagID(key)
			syscalls = syscallsByImageTagID[imageTagID]
			capabilities = capabilitiesByImageTagID[imageTagID]
		}

		outCtx := &adprotov1.ProfileContext{
			FirstSeen:      ctx.FirstSeenNano,
			LastSeen:       ctx.LastSeenNano,
			EventTypeState: make(map[uint32]*adprotov1.EventTypeState),
			Syscalls:       make([]uint32, len(syscalls)),
			Tags:           make([]string, len(ctx.Tags)),
		}
		if capabilities != nil {
			outCtx.AttemptedCapabilities = slices.Clone(capabilities.Attempted)
			outCtx.UsedCapabilities = slices.Clone(capabilities.Used)
		}
		for evtType, evtState := range ctx.EventTypeState {
			outCtx.EventTypeState[uint32(evtType)] = &adprotov1.EventTypeState{
				LastAnomalyNano:   evtState.LastAnomalyNano,
				EventProfileState: eventFilteringProfileStateToProto(evtState.State),
			}
		}
		copy(outCtx.Syscalls, syscalls)
		copy(outCtx.Tags, ctx.Tags)
		output.ProfileContexts[key] = outCtx
	}

	return &output, nil
}

// EncodeSecurityProfileProtobuf encodes a Profile to its SecurityProfile protobuf binary representation
func (p *Profile) EncodeSecurityProfileProtobuf() (*bytes.Buffer, error) {
	p.Lock()
	defer p.Unlock()

	profileProto, err := profileToSecurityProfileProto(p)
	if err != nil {
		return nil, fmt.Errorf("Error while encoding security dump: %v", err)
	}
	raw, err := profileProto.MarshalVT()
	if err != nil {
		return nil, fmt.Errorf("couldn't encode dump to `%s` format: %v", config.Profile, err)
	}
	return bytes.NewBuffer(raw), nil
}

// protoToSecurityProfile converts a SecurityProfile protobuf object to a Profile
func protoToSecurityProfile(output *Profile, input *adprotov1.SecurityProfile) {
	if input == nil {
		return
	}

	output.Metadata = mtdt.ProtoMetadataToMetadata(input.Metadata)
	output.selector = cgroupModel.ProtoToWorkloadSelector(input.Selector)
	output.isEnabled = !input.Disabled

	for key, ctx := range input.ProfileContexts {
		outCtx := &VersionContext{
			FirstSeenNano:  ctx.FirstSeen,
			LastSeenNano:   ctx.LastSeen,
			EventTypeState: make(map[model.EventType]*EventTypeState),
			Syscalls:       make([]uint32, len(ctx.Syscalls)),
			Tags:           make([]string, len(ctx.Tags)),
		}
		for evtType, evtState := range ctx.EventTypeState {
			outCtx.EventTypeState[model.EventType(evtType)] = &EventTypeState{
				LastAnomalyNano: evtState.LastAnomalyNano,
				State:           protoToState(evtState.EventProfileState),
			}
		}
		copy(outCtx.Syscalls, ctx.Syscalls)
		copy(outCtx.Tags, ctx.Tags)
		output.versionContexts[key] = outCtx
	}

	activity_tree.ProtoDecodeActivityTree(output.ActivityTree, input.Tree)
}

// LoadProtoFromFile loads proto profile from file
func LoadProtoFromFile(filepath string) (*adprotov1.SecurityProfile, error) {
	raw, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("couldn't read profile: %w", err)
	}

	pp := &adprotov1.SecurityProfile{}
	if err = pp.UnmarshalVT(raw); err != nil {
		return nil, fmt.Errorf("couldn't decode protobuf profile: %w", err)
	}

	return pp, nil
}

// DecodeSecurityProfileProtobuf decodes a SecurityProfile binary representation
func (p *Profile) DecodeSecurityProfileProtobuf(reader io.Reader) error {
	p.Lock()
	defer p.Unlock()

	raw, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("couldn't open security profile file: %w", err)
	}

	inter := &adprotov1.SecurityProfile{}
	if err := inter.UnmarshalVT(raw); err != nil {
		return fmt.Errorf("couldn't decode protobuf activity dump file: %w", err)
	}

	protoToSecurityProfile(p, inter)

	return nil
}

// unionSyscalls returns the sorted set of every syscall observed across all image tag IDs.
func unionSyscalls(byTag map[uint64][]uint32) []uint32 {
	if len(byTag) == 0 {
		return nil
	}
	set := make(map[uint32]struct{})
	for _, syscalls := range byTag {
		for _, s := range syscalls {
			set[s] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]uint32, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

// unionCapabilities returns the sorted attempted/used sets aggregated across all image tag IDs.
func unionCapabilities(byTag map[uint64]*activity_tree.CapabilityRollup) *activity_tree.CapabilityRollup {
	if len(byTag) == 0 {
		return nil
	}
	attempted := make(map[uint64]struct{})
	used := make(map[uint64]struct{})
	for _, r := range byTag {
		if r == nil {
			continue
		}
		for _, c := range r.Attempted {
			attempted[c] = struct{}{}
		}
		for _, c := range r.Used {
			used[c] = struct{}{}
		}
	}
	if len(attempted) == 0 && len(used) == 0 {
		return nil
	}
	return &activity_tree.CapabilityRollup{
		Attempted: sortedUint64Set(attempted),
		Used:      sortedUint64Set(used),
	}
}

func sortedUint64Set(set map[uint64]struct{}) []uint64 {
	if len(set) == 0 {
		return nil
	}
	out := make([]uint64, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}
