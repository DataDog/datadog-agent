// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import (
	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
)

func securityProfileProtoToActivityDump(dest *ActivityDump, ad *proto.SecurityProfile) {
	if ad == nil {
		return
	}

	dest.Metadata = mtdt.ProtoMetadataToMetadata(ad.Metadata)

	dest.Tags = make([]string, len(ad.Tags))
	copy(dest.Tags, ad.Tags)
	dest.StorageRequests = make(map[config.StorageFormat][]config.StorageRequest)

	dest.ActivityTree = activity_tree.NewActivityTree(dest, "activity_dump")
	activity_tree.ProtoDecodeActivityTree(dest.ActivityTree, ad.Tree)
}
