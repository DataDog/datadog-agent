// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import (
	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
)

// ActivityDumpToSecurityProfileProto serializes an Activity Dump to a Security Profile protobuf representation
func ActivityDumpToSecurityProfileProto(input *ActivityDump) *proto.SecurityProfile {
	if input == nil {
		return nil
	}

	output := proto.SecurityProfile{
		Status:   uint32(model.AnomalyDetection + model.AutoSuppression),
		Version:  security_profile.LocalProfileVersion,
		Metadata: mtdt.MetadataToProto(&input.Metadata),
		Syscalls: input.ActivityTree.ComputeSyscallsList(),
		Tags:     make([]string, len(input.Tags)),
		Tree:     activity_tree.ActivityTreeToProto(input.ActivityTree),
	}
	copy(output.Tags, input.Tags)

	return &output
}
