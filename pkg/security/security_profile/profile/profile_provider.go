// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package profile

import (
	"context"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
)

// Provider defines a profile provider
type Provider interface {
	// Start runs the profile provider
	Start(ctx context.Context) error
	// Stop closes the profile provider
	Stop() error

	// UpdateWorkloadSelectors updates the selectors used to query profiles
	UpdateWorkloadSelectors(selectors []cgroupModel.WorkloadSelector)
	// SetOnNewProfileCallback sets the onNewProfileCallback function
	SetOnNewProfileCallback(onNewProfileCallback func(selector cgroupModel.WorkloadSelector, profile *proto.SecurityProfile))
}
