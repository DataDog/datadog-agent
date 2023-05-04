// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import (
	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

func protoToActivityDump(dest *ActivityDump, ad *adproto.SecDump) {
	if ad == nil {
		return
	}

	dest.Host = ad.Host
	dest.Service = ad.Service
	dest.Source = ad.Source
	dest.Metadata = ProtoMetadataToMetadata(ad.Metadata)

	dest.Tags = make([]string, len(ad.Tags))
	copy(dest.Tags, ad.Tags)
	dest.StorageRequests = make(map[config.StorageFormat][]config.StorageRequest)

	dest.ActivityTree = activity_tree.NewActivityTree(dest, "activity_dump")
	activity_tree.ProtoDecodeActivityTree(dest.ActivityTree, ad.Tree)
}

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
