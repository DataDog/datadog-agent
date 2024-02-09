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

// SecurityProfileToProto incode a Security Profile to its protobuf representation
func SecurityProfileToProto(input *SecurityProfile) *proto.SecurityProfile {
	if input == nil {
		return nil
	}

	output := proto.SecurityProfile{
		Version:  input.Version,
		Metadata: mtdt.ToProto(&input.Metadata),
		Syscalls: input.Syscalls,
		Tags:     make([]string, len(input.Tags)),
		Tree:     activity_tree.ToProto(input.ActivityTree),
	}
	copy(output.Tags, input.Tags)

	return &output
}
