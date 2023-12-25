// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package startcmd holds the start command of CWS injector
package startcmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activitytree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

// SendSyscallMsg handles gRPC messages
func (p *ProfilerContext) SendSyscallMsg(_ context.Context, syscallMsg *ebpfless.SyscallMsg) (*ebpfless.Response, error) {
	event := model.NewDefaultEvent() // field handlers not set

	switch syscallMsg.Type {
	case ebpfless.SyscallType_Exec:
		entry := p.processResolver.AddExecEntry(syscallMsg.PID, syscallMsg.Exec.Filename, syscallMsg.Exec.Args, syscallMsg.Exec.Envs, syscallMsg.ContainerContext.ID)
		event.Type = uint32(model.ExecEventType)
		event.Exec.Process = &entry.Process
		// DEBUG
		fmt.Printf("EXEC: %v %v %v %v\n", syscallMsg.PID, syscallMsg.Exec.Filename, syscallMsg.Exec.Args, syscallMsg.ContainerContext.ID)
		ancestor := entry.ProcessContext.Ancestor
		for ancestor != nil {
			fmt.Printf("  - from: %v %v %v\n", ancestor.Pid, ancestor.FileEvent.PathnameStr, ancestor.Argv)
			ancestor = ancestor.ProcessContext.Ancestor
		}
	case ebpfless.SyscallType_Fork:
		fmt.Printf("FORK: %v -> %v\n", syscallMsg.Fork.PPID, syscallMsg.PID)
		p.processResolver.AddForkEntry(syscallMsg.PID, syscallMsg.Fork.PPID)
		event.Type = uint32(model.ForkEventType)
	case ebpfless.SyscallType_Open:
		fmt.Printf("OPEN: %v\n", syscallMsg.Open.Filename)
		event.Type = uint32(model.FileOpenEventType)
		event.Open.File.PathnameStr = syscallMsg.Open.Filename
		event.Open.File.BasenameStr = filepath.Base(syscallMsg.Open.Filename)
		event.Open.Flags = syscallMsg.Open.Flags
		event.Open.Mode = syscallMsg.Open.Mode
	default:
		return &ebpfless.Response{}, nil
	}

	// container context
	event.ContainerContext.ID = syscallMsg.ContainerContext.ID
	event.ContainerContext.CreatedAt = syscallMsg.ContainerContext.CreatedAt
	imageName := syscallMsg.ContainerContext.Name
	if imageName == "" {
		imageName = "host"
	}
	imageTag := syscallMsg.ContainerContext.Tag
	if imageTag == "" {
		imageTag = "latest"
	}
	event.ContainerContext.Tags = []string{
		"image_name:" + imageName,
		"image_tag:" + imageTag,
	}

	// use ProcessCacheEntry process context as process context
	event.ProcessCacheEntry = p.processResolver.Resolve(syscallMsg.PID)
	if event.ProcessCacheEntry == nil {
		event.ProcessCacheEntry = model.NewPlaceholderProcessCacheEntry(syscallMsg.PID, syscallMsg.PID, false)
	}
	event.ProcessContext = &event.ProcessCacheEntry.ProcessContext

	_, err := p.dump.ActivityTree.Insert(event, true /*insertMissingProcesses*/, activitytree.Runtime, nil /*resolvers*/)
	if err != nil {
		fmt.Printf("dump insertion failed: %v\n", err)
	}
	return &ebpfless.Response{}, nil
}
