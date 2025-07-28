// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
)

// ToSecurityActivityDumpMessage returns a pointer to a SecurityActivityDumpMessage
func (p *Profile) ToSecurityActivityDumpMessage(timeout time.Duration, storageRequests map[config.StorageFormat][]config.StorageRequest) *api.ActivityDumpMessage {
	p.m.Lock()
	defer p.m.Unlock()
	var storage []*api.StorageRequestMessage
	for _, requests := range storageRequests {
		for _, request := range requests {
			storage = append(storage, request.ToStorageRequestMessage(p.Metadata.Name))
		}
	}

	msg := &api.ActivityDumpMessage{
		Host:    p.Header.Host,
		Source:  p.Header.Source,
		Service: p.Header.Service,
		Tags:    p.tags,
		Storage: storage,
		Metadata: &api.MetadataMessage{
			AgentVersion:      p.Metadata.AgentVersion,
			AgentCommit:       p.Metadata.AgentCommit,
			KernelVersion:     p.Metadata.KernelVersion,
			LinuxDistribution: p.Metadata.LinuxDistribution,
			Name:              p.Metadata.Name,
			ProtobufVersion:   p.Metadata.ProtobufVersion,
			DifferentiateArgs: p.Metadata.DifferentiateArgs,
			ContainerID:       string(p.Metadata.ContainerID),
			CGroupID:          string(p.Metadata.CGroupContext.CGroupID),
			CGroupManager:     p.Metadata.CGroupContext.CGroupFlags.GetCGroupManager().String(),
			Start:             p.Metadata.Start.Format(time.RFC822),
			Timeout:           timeout.String(),
			Size:              p.Metadata.Size,
			Arch:              p.Metadata.Arch,
		},
		DNSNames: p.Header.DNSNames.Keys(),
	}
	if p.ActivityTree != nil {
		msg.Stats = &api.ActivityTreeStatsMessage{
			ProcessNodesCount: p.ActivityTree.Stats.ProcessNodes,
			FileNodesCount:    p.ActivityTree.Stats.FileNodes,
			DNSNodesCount:     p.ActivityTree.Stats.DNSNodes,
			SocketNodesCount:  p.ActivityTree.Stats.SocketNodes,
			IMDSNodesCount:    p.ActivityTree.Stats.IMDSNodes,
			SyscallNodesCount: p.ActivityTree.Stats.SyscallNodes,
			FlowNodesCount:    p.ActivityTree.Stats.FlowNodes,
			ApproximateSize:   p.ActivityTree.Stats.ApproximateSize(),
		}
	}
	return msg
}

// NewProfileFromActivityDumpMessage returns a new Profile from a ActivityDumpMessage.
func NewProfileFromActivityDumpMessage(msg *api.ActivityDumpMessage) (*Profile, map[config.StorageFormat][]config.StorageRequest, error) {
	metadata := msg.GetMetadata()
	if metadata == nil {
		return nil, nil, fmt.Errorf("couldn't create new Profile: missing activity dump metadata")
	}

	startTime, err := time.Parse(time.RFC822, metadata.GetStart())
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't parse start time [%s]: %w", metadata.GetStart(), err)
	}
	timeout, err := time.ParseDuration(metadata.GetTimeout())
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't parse timeout [%s]: %w", metadata.GetTimeout(), err)
	}

	p := New(WithDifferentiateArgs(metadata.GetDifferentiateArgs()))
	p.Header.Host = msg.GetHost()
	p.Header.Service = msg.GetService()
	p.Header.Source = msg.GetSource()
	p.AddTags(msg.GetTags())
	p.Metadata = mtdt.Metadata{
		AgentVersion:      metadata.GetAgentVersion(),
		AgentCommit:       metadata.GetAgentCommit(),
		KernelVersion:     metadata.GetKernelVersion(),
		LinuxDistribution: metadata.GetLinuxDistribution(),
		Name:              metadata.GetName(),
		ProtobufVersion:   metadata.GetProtobufVersion(),
		DifferentiateArgs: metadata.GetDifferentiateArgs(),
		ContainerID:       containerutils.ContainerID(metadata.GetContainerID()),
		CGroupContext: model.CGroupContext{
			CGroupID:      containerutils.CGroupID(metadata.GetCGroupID()),
			CGroupManager: metadata.GetCGroupManager(),
		},
		Start: startTime,
		End:   startTime.Add(timeout),
		Size:  metadata.GetSize(),
		Arch:  metadata.GetArch(),
	}
	p.Header.DNSNames = utils.NewStringKeys(msg.GetDNSNames())

	// parse requests from message
	storageRequests := make(map[config.StorageFormat][]config.StorageRequest)
	for _, request := range msg.GetStorage() {
		storageType, err := config.ParseStorageType(request.GetType())
		if err != nil {
			// invalid storage type, ignore
			continue
		}
		storageFormat, err := config.ParseStorageFormat(request.GetFormat())
		if err != nil {
			// invalid storage type, ignore
			continue
		}
		storageRequests[storageFormat] = append(storageRequests[storageFormat], config.NewStorageRequest(
			storageType,
			storageFormat,
			request.GetCompression(),
			filepath.Base(request.File),
		))
	}

	// are we missing the storage requests and the load controller config?

	return p, storageRequests, nil
}

// ToSecurityProfileMessage returns a SecurityProfileMessage filled with the content of the current Security Profile
func (p *Profile) ToSecurityProfileMessage(timeResolver *ktime.Resolver) *api.SecurityProfileMessage {
	p.m.Lock()
	defer p.m.Unlock()

	// construct the list of image tags for this profile
	imageTags := ""
	for key := range p.versionContexts {
		if imageTags != "" {
			imageTags = imageTags + ","
		}
		imageTags = imageTags + key
	}

	msg := &api.SecurityProfileMessage{
		LoadedInKernel:          p.LoadedInKernel.Load(),
		LoadedInKernelTimestamp: timeResolver.ResolveMonotonicTimestamp(p.LoadedNano.Load()).String(),
		Selector: &api.WorkloadSelectorMessage{
			Name: p.selector.Image,
			Tag:  imageTags,
		},
		ProfileCookie: p.profileCookie,
		Metadata: &api.MetadataMessage{
			Name: p.Metadata.Name,
		},
		ProfileGlobalState: p.getGlobalState().String(),
		ProfileContexts:    make(map[string]*api.ProfileContextMessage),
	}
	for imageTag, ctx := range p.versionContexts {
		msgCtx := &api.ProfileContextMessage{
			FirstSeen:      ctx.FirstSeenNano,
			LastSeen:       ctx.LastSeenNano,
			EventTypeState: make(map[string]*api.EventTypeState),
			Tags:           ctx.Tags,
		}
		for et, state := range ctx.EventTypeState {
			msgCtx.EventTypeState[et.String()] = &api.EventTypeState{
				LastAnomalyNano:   state.LastAnomalyNano,
				EventProfileState: state.State.String(),
			}
		}
		msg.ProfileContexts[imageTag] = msgCtx
	}

	if p.ActivityTree != nil {
		msg.Stats = &api.ActivityTreeStatsMessage{
			ProcessNodesCount: p.ActivityTree.Stats.ProcessNodes,
			FileNodesCount:    p.ActivityTree.Stats.FileNodes,
			DNSNodesCount:     p.ActivityTree.Stats.DNSNodes,
			SocketNodesCount:  p.ActivityTree.Stats.SocketNodes,
			ApproximateSize:   p.ActivityTree.Stats.ApproximateSize(),
		}
	}

	for _, evt := range p.eventTypes {
		msg.EventTypes = append(msg.EventTypes, evt.String())
	}

	for _, inst := range p.Instances {
		msg.Instances = append(msg.Instances, &api.InstanceMessage{
			ContainerID: string(inst.ContainerID),
			Tags:        inst.Tags,
		})
	}
	return msg
}
