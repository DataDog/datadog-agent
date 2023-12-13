// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rconfig holds rconfig related files
package rconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	"github.com/DataDog/datadog-go/v5/statsd"

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

// ProfileConfig defines a profile config
type ProfileConfig struct {
	Tags    []string
	Profile []byte
}

// RCProfileProvider defines a RC profile provider
type RCProfileProvider struct {
	sync.RWMutex

	client *remote.Client

	onNewProfileCallback func(selector cgroupModel.WorkloadSelector, profile *proto.SecurityProfile)
}

// Stop stops the client
func (r *RCProfileProvider) Stop() error {
	r.client.Close()
	return nil
}

func (r *RCProfileProvider) rcProfilesUpdateCallback(configs map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
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

// Start starts the Remote Config profile provider and subscribes to updates
func (r *RCProfileProvider) Start(ctx context.Context) error {
	log.Info("remote-config profile provider started")

	r.client.Start()
	r.client.Subscribe(state.ProductCWSProfiles, r.rcProfilesUpdateCallback)

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

// SendStats sends the metrics of the directory provider
func (r *RCProfileProvider) SendStats(_ statsd.ClientInterface) error {
	return nil
}

// NewRCProfileProvider returns a new Remote Config based policy provider
func NewRCProfileProvider() (*RCProfileProvider, error) {
	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to parse agent version: %v", err)
	}

	c, err := remote.NewUnverifiedGRPCClient(agentName, agentVersion.String(), []data.Product{data.ProductCWSProfile}, securityAgentRCPollInterval)
	if err != nil {
		return nil, err
	}

	r := &RCProfileProvider{
		client: c,
	}

	return r, nil
}
