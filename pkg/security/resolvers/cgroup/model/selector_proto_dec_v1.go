// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
)

// ProtoToWorkloadSelector decodes a Selector structure
func ProtoToWorkloadSelector(selector *proto.ProfileSelector) WorkloadSelector {
	if selector == nil {
		return WorkloadSelector{}
	}

	return WorkloadSelector{
		Image: selector.GetImageName(),
		Tag:   selector.GetImageTag(),
	}
}
