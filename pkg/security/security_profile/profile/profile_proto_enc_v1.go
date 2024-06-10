// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
)

// EventFilteringProfileStateToProto convert a profile state to a proto one
func EventFilteringProfileStateToProto(efr model.EventFilteringProfileState) proto.EventProfileState {
	switch efr {
	case model.NoProfile:
		return proto.EventProfileState_NO_PROFILE
	case model.ProfileAtMaxSize:
		return proto.EventProfileState_PROFILE_AT_MAX_SIZE
	case model.UnstableEventType:
		return proto.EventProfileState_UNSTABLE_PROFILE
	case model.StableEventType:
		return proto.EventProfileState_STABLE_PROFILE
	case model.AutoLearning:
		return proto.EventProfileState_AUTO_LEARNING
	case model.WorkloadWarmup:
		return proto.EventProfileState_WORKLOAD_WARMUP
	}
	return proto.EventProfileState_NO_PROFILE
}

// SecurityProfileToProto incode a Security Profile to its protobuf representation
func SecurityProfileToProto(input *SecurityProfile) *proto.SecurityProfile {
	if input == nil {
		return nil
	}

	output := proto.SecurityProfile{
		Metadata:        mtdt.ToProto(&input.Metadata),
		ProfileContexts: make(map[string]*proto.ProfileContext),
		Tree:            activity_tree.ToProto(input.ActivityTree),
		Selector:        cgroupModel.WorkloadSelectorToProto(&input.selector),
	}

	for key, ctx := range input.versionContexts {
		outCtx := &proto.ProfileContext{
			FirstSeen:      ctx.firstSeenNano,
			LastSeen:       ctx.lastSeenNano,
			EventTypeState: make(map[uint32]*proto.EventTypeState),
			Syscalls:       make([]uint32, len(ctx.Syscalls)),
			Tags:           make([]string, len(ctx.Tags)),
		}
		for evtType, evtState := range ctx.eventTypeState {
			outCtx.EventTypeState[uint32(evtType)] = &proto.EventTypeState{
				LastAnomalyNano:   evtState.lastAnomalyNano,
				EventProfileState: EventFilteringProfileStateToProto(evtState.state),
			}
		}
		copy(outCtx.Syscalls, ctx.Syscalls)
		copy(outCtx.Tags, ctx.Tags)
		output.ProfileContexts[key] = outCtx
	}

	return &output
}
