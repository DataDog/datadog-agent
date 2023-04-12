// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package profile

import (
	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/dump"
)

func protoToSecurityProfile(output *SecurityProfile, input *proto.SecurityProfile) {
	if input == nil {
		return
	}

	output.Status = Status(input.Status)
	output.Version = input.Version
	output.Metadata = dump.ProtoMetadataToMetadata(input.Metadata)

	output.Tags = make([]string, len(input.Tags))
	copy(output.Tags, input.Tags)

	output.Syscalls = make([]uint32, len(input.Syscalls))
	copy(output.Syscalls, input.Syscalls)

	output.ProcessActivityTree = make([]*dump.ProcessActivityNode, 0, len(input.Tree))
	for _, tree := range input.Tree {
		output.ProcessActivityTree = append(output.ProcessActivityTree, dump.ProtoDecodeProcessActivityNode(tree))
	}
}
