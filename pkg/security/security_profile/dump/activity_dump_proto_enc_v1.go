// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import (
	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

func activityDumpToProto(ad *ActivityDump) *adproto.SecDump {
	if ad == nil {
		return nil
	}

	pad := adproto.SecDumpFromVTPool()
	*pad = adproto.SecDump{
		Host:    ad.Host,
		Service: ad.Service,
		Source:  ad.Source,

		Metadata: adMetadataToProto(&ad.Metadata),

		Tags: make([]string, len(ad.Tags)),
		Tree: activity_tree.ActivityTreeToProto(ad.ActivityTree),
	}

	copy(pad.Tags, ad.Tags)

	return pad
}

func adMetadataToProto(meta *Metadata) *adproto.Metadata {
	if meta == nil {
		return nil
	}

	pmeta := &adproto.Metadata{
		AgentVersion:      meta.AgentVersion,
		AgentCommit:       meta.AgentCommit,
		KernelVersion:     meta.KernelVersion,
		LinuxDistribution: meta.LinuxDistribution,

		Name:              meta.Name,
		ProtobufVersion:   meta.ProtobufVersion,
		DifferentiateArgs: meta.DifferentiateArgs,
		Comm:              meta.Comm,
		ContainerId:       meta.ContainerID,
		Start:             activity_tree.TimestampToProto(&meta.Start),
		End:               activity_tree.TimestampToProto(&meta.End),
		Size:              meta.Size,
		Arch:              meta.Arch,
		Serialization:     meta.Serialization,
	}

	return pmeta
}
