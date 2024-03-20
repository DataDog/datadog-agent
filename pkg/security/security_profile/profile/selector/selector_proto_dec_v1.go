// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package selector holds selector related files
package selector

import (
	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
)

// ProtoToWorkloadSelector decodes a Selector structure
func ProtoToWorkloadSelector(selector *proto.ProfileSelector) cgroupModel.WorkloadSelector {
	if selector == nil {
		return cgroupModel.WorkloadSelector{}
	}

	return cgroupModel.WorkloadSelector{
		Image: selector.GetImageName(),
		Tag:   selector.GetImageTag(),
	}
}
