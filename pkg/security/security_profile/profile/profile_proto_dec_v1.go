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

// ProtoToSecurityProfile decodes a Security Profile from its protobuf representation
func ProtoToSecurityProfile(output *SecurityProfile, pathsReducer *activity_tree.PathsReducer, input *proto.SecurityProfile) {
	if input == nil {
		return
	}

	output.Metadata = mtdt.ProtoMetadataToMetadata(input.Metadata)
	output.selector = cgroupModel.ProtoToWorkloadSelector(input.Selector)

	for key, ctx := range input.ProfileContexts {
		outCtx := &VersionContext{
			firstSeenNano:  ctx.FirstSeen,
			lastSeenNano:   ctx.LastSeen,
			eventTypeState: make(map[model.EventType]*EventTypeState),
			Syscalls:       make([]uint32, len(ctx.Syscalls)),
			Tags:           make([]string, len(ctx.Tags)),
		}
		for evtType, evtState := range ctx.EventTypeState {
			outCtx.eventTypeState[model.EventType(evtType)] = &EventTypeState{
				lastAnomalyNano: evtState.LastAnomalyNano,
				state:           ProtoToState(evtState.EventProfileState),
			}
		}
		copy(outCtx.Syscalls, ctx.Syscalls)
		copy(outCtx.Tags, ctx.Tags)
		output.versionContexts[key] = outCtx
	}

	output.ActivityTree = activity_tree.NewActivityTree(output, pathsReducer, "security_profile")
	activity_tree.ProtoDecodeActivityTree(output.ActivityTree, input.Tree)
}
