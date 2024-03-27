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
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
)

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
				EventProfileState: evtState.state.toProto(),
			}
		}
		copy(outCtx.Syscalls, ctx.Syscalls)
		copy(outCtx.Tags, ctx.Tags)
		output.ProfileContexts[key] = outCtx
	}

	return &output
}
