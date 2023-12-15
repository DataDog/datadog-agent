// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dump holds dump related files
package dump

import (
	adproto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
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

		Metadata: mtdt.ToProto(&ad.Metadata),

		Tags: make([]string, len(ad.Tags)),
		Tree: activity_tree.ToProto(ad.ActivityTree),
	}

	copy(pad.Tags, ad.Tags)

	return pad
}
