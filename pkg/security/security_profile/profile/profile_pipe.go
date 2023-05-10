// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package profile

import (
	"context"
	"sync"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

type PipeProvider struct {
	sync.Mutex
	c                    chan *proto.SecurityProfile
	onNewProfileCallback func(selector cgroupModel.WorkloadSelector, profile *proto.SecurityProfile)
	// selectors is used to select the profiles we currently care about
	selectors []cgroupModel.WorkloadSelector
}

func NewPipeProvider(pipeBufferSize int) *PipeProvider {
	return &PipeProvider{
		c: make(chan *proto.SecurityProfile, pipeBufferSize),
	}
}

func (pipe *PipeProvider) filterProfile(workloadSelector cgroupModel.WorkloadSelector, profile *proto.SecurityProfile) {
	pipe.Lock()
	defer pipe.Unlock()

	if pipe.onNewProfileCallback == nil {
		return
	}

	// check if this profile matches a workload selector
	for _, selector := range pipe.selectors {
		if workloadSelector.Match(selector) {
			pipe.onNewProfileCallback(workloadSelector, profile)
		}
	}
}

// Start runs the pipe provider
func (pipe *PipeProvider) Start(ctx context.Context) error {
	go func() {
		for profile := range pipe.c {
			workloadSelector, err := cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", profile.Tags), utils.GetTagValue("image_tag", profile.Tags))
			if err != nil {
				continue
			}
			seclog.Debugf("security profile %s (version: %s status: %s) received from activity dump pipe", workloadSelector, profile.Version, model.Status(profile.Status))
			pipe.filterProfile(workloadSelector, profile)
		}
	}()
	return nil
}

// Stop closes the pipe provider
func (pipe *PipeProvider) Stop() error {
	close(pipe.c)
	return nil
}

// UpdateWorkloadSelectors updates the selectors used to filter profiles
func (pipe *PipeProvider) UpdateWorkloadSelectors(selectors []cgroupModel.WorkloadSelector) {
	pipe.Lock()
	defer pipe.Unlock()
	pipe.selectors = selectors
}

// SetOnNewProfileCallback sets the onNewProfileCallback function
func (pipe *PipeProvider) SetOnNewProfileCallback(onNewProfileCallback func(selector cgroupModel.WorkloadSelector, profile *proto.SecurityProfile)) {
	pipe.Lock()
	defer pipe.Unlock()
	pipe.onNewProfileCallback = onNewProfileCallback
}

func (pipe *PipeProvider) QueueProfileProto(profile *proto.SecurityProfile) {
	select {
	case pipe.c <- profile:
	default:
	}
}
