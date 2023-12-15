// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package metadata holds metadata related files
package metadata

import (
	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

// ProtoMetadataToMetadata decodes a Metadata structure
func ProtoMetadataToMetadata(meta *adproto.Metadata) Metadata {
	if meta == nil {
		return Metadata{}
	}

	return Metadata{
		AgentVersion:      meta.AgentVersion,
		AgentCommit:       meta.AgentCommit,
		KernelVersion:     meta.KernelVersion,
		LinuxDistribution: meta.LinuxDistribution,
		Arch:              meta.Arch,

		Name:              meta.Name,
		ProtobufVersion:   meta.ProtobufVersion,
		DifferentiateArgs: meta.DifferentiateArgs,
		Comm:              meta.Comm,
		ContainerID:       meta.ContainerId,
		Start:             activity_tree.ProtoDecodeTimestamp(meta.Start),
		End:               activity_tree.ProtoDecodeTimestamp(meta.End),
		Size:              meta.Size,
		Serialization:     meta.GetSerialization(),
	}
}
