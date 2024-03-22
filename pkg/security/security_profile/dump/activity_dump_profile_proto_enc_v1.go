// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dump holds dump related files
package dump

import (
	"errors"
	"time"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	timeResolver "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
)

// ActivityDumpToSecurityProfileProto serializes an Activity Dump to a Security Profile protobuf representation
func ActivityDumpToSecurityProfileProto(input *ActivityDump) (*proto.SecurityProfile, error) {
	if input == nil {
		return nil, errors.New("imput == nil")
	}
	selector := input.GetWorkloadSelector()
	if selector == nil {
		return nil, errors.New("can't get dump selector, tags shouldn't be resolved yet")
	}

	output := &proto.SecurityProfile{
		Metadata:        mtdt.ToProto(&input.Metadata),
		ProfileContexts: make(map[string]*proto.ProfileContext),
		Tree:            activity_tree.ToProto(input.ActivityTree),
	}
	timeResolver, err := timeResolver.NewResolver()
	if err != nil {
		return nil, errors.New("can't init time resolver")
	}
	ts := uint64(timeResolver.ComputeMonotonicTimestamp(time.Now()))
	ctx := &proto.ProfileContext{
		Syscalls:  input.ActivityTree.ComputeSyscallsList(),
		Tags:      make([]string, len(input.Tags)),
		FirstSeen: ts,
		LastSeen:  ts,
	}
	copy(ctx.Tags, input.Tags)
	output.ProfileContexts[selector.Tag] = ctx

	return output, nil
}
