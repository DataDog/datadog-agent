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

	adprotov1 "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/securitycontext"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
)

func securityContextToProto(sc *securitycontext.SecurityContext) *adprotov1.SecurityContext {
	if sc == nil {
		return nil
	}
	return &adprotov1.SecurityContext{
		Privileged:               sc.Privileged,
		Seccomp:                  seccompToProto(sc.Seccomp),
		CapabilitiesAdd:          sc.CapabilitiesAdd,
		CapabilitiesDrop:         sc.CapabilitiesDrop,
		RunAsNonRoot:             copyBoolPtr(sc.RunAsNonRoot),
		AllowPrivilegeEscalation: copyBoolPtr(sc.AllowPrivilegeEscalation),
		ReadOnlyRootFilesystem:   copyBoolPtr(sc.ReadOnlyRootFilesystem),
	}
}

func protoToSecurityContext(sc *adprotov1.SecurityContext) *securitycontext.SecurityContext {
	if sc == nil {
		return nil
	}
	return &securitycontext.SecurityContext{
		Privileged:               sc.GetPrivileged(),
		Seccomp:                  seccompFromProto(sc.GetSeccomp()),
		CapabilitiesAdd:          sc.GetCapabilitiesAdd(),
		CapabilitiesDrop:         sc.GetCapabilitiesDrop(),
		RunAsNonRoot:             copyBoolPtr(sc.RunAsNonRoot),
		AllowPrivilegeEscalation: copyBoolPtr(sc.AllowPrivilegeEscalation),
		ReadOnlyRootFilesystem:   copyBoolPtr(sc.ReadOnlyRootFilesystem),
	}
}

// copyBoolPtr returns a fresh *bool with src's value, or nil when src is nil,
// so callers never alias the sender's storage across the wire boundary.
func copyBoolPtr(src *bool) *bool {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}

// seccompToProto drops LocalhostProfile for any type != Localhost to keep the
// wire invariant tight even when the caller hand-builds a bogus struct.
func seccompToProto(s *securitycontext.SeccompProfile) *adprotov1.SeccompProfile {
	if s == nil {
		return nil
	}
	out := &adprotov1.SeccompProfile{Type: seccompTypeToProto(s.Type)}
	if s.Type == securitycontext.SeccompLocalhost && s.LocalhostProfile != "" {
		lp := s.LocalhostProfile
		out.LocalhostProfile = &lp
	}
	return out
}

// seccompFromProto returns nil for the UNKNOWN type so absence and "no data"
// look the same to consumers.
func seccompFromProto(s *adprotov1.SeccompProfile) *securitycontext.SeccompProfile {
	if s == nil {
		return nil
	}
	t := seccompTypeFromProto(s.GetType())
	if t == securitycontext.SeccompUnknown {
		return nil
	}
	out := &securitycontext.SeccompProfile{Type: t}
	if t == securitycontext.SeccompLocalhost {
		out.LocalhostProfile = s.GetLocalhostProfile()
	}
	return out
}

func seccompTypeToProto(t securitycontext.SeccompProfileType) adprotov1.SeccompProfile_Type {
	switch t {
	case securitycontext.SeccompUnconfined:
		return adprotov1.SeccompProfile_TYPE_UNCONFINED
	case securitycontext.SeccompRuntimeDefault:
		return adprotov1.SeccompProfile_TYPE_RUNTIME_DEFAULT
	case securitycontext.SeccompLocalhost:
		return adprotov1.SeccompProfile_TYPE_LOCALHOST
	default:
		return adprotov1.SeccompProfile_TYPE_UNKNOWN
	}
}

func seccompTypeFromProto(t adprotov1.SeccompProfile_Type) securitycontext.SeccompProfileType {
	switch t {
	case adprotov1.SeccompProfile_TYPE_UNCONFINED:
		return securitycontext.SeccompUnconfined
	case adprotov1.SeccompProfile_TYPE_RUNTIME_DEFAULT:
		return securitycontext.SeccompRuntimeDefault
	case adprotov1.SeccompProfile_TYPE_LOCALHOST:
		return securitycontext.SeccompLocalhost
	default:
		return securitycontext.SeccompUnknown
	}
}

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
		Metadata:        mtdt.ToProto(&p.Metadata),
		Tags:            make([]string, len(p.tags)),
		Tree:            activity_tree.ToProto(p.ActivityTree),
		SecurityContext: securityContextToProto(p.SecurityContext),
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
	p.SecurityContext = protoToSecurityContext(ad.SecurityContext)

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
		SecurityContext: securityContextToProto(p.SecurityContext),
	}

	for key, ctx := range p.versionContexts {
		outCtx := &adprotov1.ProfileContext{
			FirstSeen:      ctx.FirstSeenNano,
			LastSeen:       ctx.LastSeenNano,
			EventTypeState: make(map[uint32]*adprotov1.EventTypeState),
			Syscalls:       make([]uint32, len(ctx.Syscalls)),
			Tags:           make([]string, len(ctx.Tags)),
		}
		for evtType, evtState := range ctx.EventTypeState {
			outCtx.EventTypeState[uint32(evtType)] = &adprotov1.EventTypeState{
				LastAnomalyNano:   evtState.LastAnomalyNano,
				EventProfileState: eventFilteringProfileStateToProto(evtState.State),
			}
		}
		copy(outCtx.Syscalls, ctx.Syscalls)
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
	output.SecurityContext = protoToSecurityContext(input.SecurityContext)

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
