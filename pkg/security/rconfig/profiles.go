// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package rconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// image name/image tag separator
	separator = ":::"
)

type ProfileConfig struct {
	Tags    []string
	Profile []byte
}

type RCProfileProvider struct {
	sync.RWMutex

	client      *remote.Client
	configState *remote.AgentConfigState

	onNewProfileCallback func(selector cgroupModel.WorkloadSelector, profile *proto.SecurityProfile)
}

// Close stops the client
func (r *RCProfileProvider) Stop() error {
	r.client.Close()
	return nil
}

func (r *RCProfileProvider) rcProfilesUpdateCallback(configs map[string]state.RawConfig) {
	for _, config := range configs {
		var profCfg ProfileConfig
		if err := json.Unmarshal(config.Config, &profCfg); err != nil {
			log.Errorf("couldn't decode json profile: %s", err)
			continue
		}

		profile := &proto.SecurityProfile{}
		if err := profile.UnmarshalVT([]byte(profCfg.Profile)); err != nil {
			log.Errorf("couldn't decode protobuf profile: %s", err)
			continue
		}

		imageName := utils.GetTagValue("image_name", profile.Tags)
		imageTag := utils.GetTagValue("image_tag", profile.Tags)

		if imageName == "" {
			log.Errorf("no image name: %v", profile.Tags)
			continue
		}

		if imageTag == "" {
			imageTag = "latest"
			profile.Tags = append(profile.Tags, "image_tag:"+imageTag)
		}

		selector, err := cgroupModel.NewWorkloadSelector(imageName, imageTag)
		if err != nil {
			log.Errorf("selector error %s/%s: %v", imageName, imageTag, err)
			continue
		}

		log.Tracef("got a new profile for %v : %v", selector, profile)

		r.onNewProfileCallback(selector, profile)
	}
}

func (r *RCProfileProvider) rcAgentConfigCallback(update map[string]state.RawConfig) {
	mergedConfig, err := remote.MergeRCAgentConfig(r.client, updates)
	if err != nil {
		return
	}

	if len(mergedConfig.LogLevel) > 0 {
		pkglog.Infof("Changing log level of the trace-agent to %s through remote config", mergedConfig.LogLevel)
		// Get the current log level
		var newFallback seelog.LogLevel
		newFallback, err = pkglog.GetLogLevel()
		if err == nil {
			r.configState.FallbackLogLevel = newFallback.String()
			err = settings.SetRuntimeSetting("log_level", mergedConfig.LogLevel)
			r.configState.LatestLogLevel = mergedConfig.LogLevel
		}
	} else {
		var currentLogLevel seelog.LogLevel
		currentLogLevel, err = pkglog.GetLogLevel()
		if err == nil && currentLogLevel.String() == r.configState.LatestLogLevel {
			pkglog.Infof("Removing remote-config log level override, falling back to %s", r.configState.FallbackLogLevel)
			err = settings.SetRuntimeSetting("log_level", r.configState.FallbackLogLevel)
		}
	}

	// Apply the new status to all configs
	for cfgPath := range updates {
		if err == nil {
			rc.client.UpdateApplyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		} else {
			rc.client.UpdateApplyStatus(cfgPath, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
	}
}

// Start starts the Remote Config profile provider and subscribes to updates
func (r *RCProfileProvider) Start(ctx context.Context) error {
	log.Info("remote-config profile provider started")

	r.client.Start()
	r.client.Subscribe(state.ProductCWSProfiles, r.rcProfilesUpdateCallback)
	r.client.Subscribe(state.ProductAgentConfig, r.rcAgentConfigCallback)

	go func() {
		<-ctx.Done()
		_ = r.Stop()
	}()

	return nil
}

func selectorToTag(selector *cgroupModel.WorkloadSelector) string {
	return selector.Image + separator + selector.Tag
}

// UpdateWorkloadSelectors updates the selectors used to query profiles
func (r *RCProfileProvider) UpdateWorkloadSelectors(selectors []cgroupModel.WorkloadSelector) {
	r.Lock()
	defer r.Unlock()

	log.Tracef("updating workload selector: %v", selectors)

	var tags []string

	for _, selector := range selectors {
		tags = append(tags, selectorToTag(&selector))
	}

	r.client.SetCWSWorkloads(tags)
}

// SetOnNewProfileCallback sets the onNewProfileCallback function
func (r *RCProfileProvider) SetOnNewProfileCallback(onNewProfileCallback func(selector cgroupModel.WorkloadSelector, profile *proto.SecurityProfile)) {
	r.onNewProfileCallback = onNewProfileCallback
}

// NewRCPolicyProvider returns a new Remote Config based policy provider
func NewRCProfileProvider() (*RCProfileProvider, error) {
	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to parse agent version: %v", err)
	}

	c, err := remote.NewUnverifiedGRPCClient(
		agentName,
		agentVersion.String(),
		[]data.Product{data.ProductCWSProfile, data.ProductAgentConfig},
		securityAgentRCPollInterval,
	)
	if err != nil {
		return nil, err
	}

	level, err := pkglog.GetLogLevel()
	if err != nil {
		return nil, err
	}

	r := &RCProfileProvider{
		client: c,
		configState: &remote.AgentConfigState{
			level.String(),
		},
	}

	return r, nil
}
