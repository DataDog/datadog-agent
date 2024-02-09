// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
)

// ProtoToSecurityProfile decodes a Security Profile from its protobuf representation
func ProtoToSecurityProfile(output *SecurityProfile, pathsReducer *activity_tree.PathsReducer, input *proto.SecurityProfile) {
	if input == nil {
		return
	}

	output.Version = input.Version
	output.Metadata = mtdt.ProtoMetadataToMetadata(input.Metadata)

	output.Tags = make([]string, len(input.Tags))
	copy(output.Tags, input.Tags)

	output.Syscalls = make([]uint32, len(input.Syscalls))
	copy(output.Syscalls, input.Syscalls)

	output.ActivityTree = activity_tree.NewActivityTree(output, pathsReducer, "security_profile")
	activity_tree.ProtoDecodeActivityTree(output.ActivityTree, input.Tree)
}
