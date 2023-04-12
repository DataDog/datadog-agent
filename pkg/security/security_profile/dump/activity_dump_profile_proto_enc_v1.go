// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import (
	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
)

// ActivityDumpToSecurityProfileProto serializes an Activity Dump to a Security Profile protobuf representation
func ActivityDumpToSecurityProfileProto(input *ActivityDump) *proto.SecurityProfile {
	if input == nil {
		return nil
	}

	output := proto.SecurityProfile{
		Status:   2,
		Version:  "1",
		Metadata: adMetadataToProto(&input.Metadata),
		Syscalls: input.computeSyscallsList(),
		Tags:     make([]string, len(input.Tags)),
		Tree:     make([]*proto.ProcessActivityNode, 0, len(input.ProcessActivityTree)),
	}
	copy(output.Tags, input.Tags)

	for _, tree := range input.ProcessActivityTree {
		output.Tree = append(output.Tree, processActivityNodeToProto(tree))
	}

	return &output
}
