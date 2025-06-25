// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// SyscallNode is used to store a syscall node
type SyscallNode struct {
	ImageTags      []string
	GenerationType NodeGenerationType
	Syscall        int
	FirstSeen      time.Time
	LastSeen       time.Time
}

func (sn *SyscallNode) appendImageTag(imageTag string) {
	sn.ImageTags, _ = AppendIfNotPresent(sn.ImageTags, imageTag)
}

func (sn *SyscallNode) evictImageTag(imageTag string) bool {
	imageTags, removed := removeImageTagFromList(sn.ImageTags, imageTag)
	if !removed {
		return false
	}
	if len(imageTags) == 0 {
		return true
	}
	sn.ImageTags = imageTags
	return false
}

func (sn *SyscallNode) updateTimes(event *model.Event) {
	eventTime := event.ResolveEventTime()
	if sn.FirstSeen.IsZero() {
		sn.FirstSeen = eventTime
		sn.LastSeen = eventTime
	} else {
		sn.LastSeen = eventTime
	}
}

// NewSyscallNode returns a new SyscallNode instance
func NewSyscallNode(syscall int, imageTag string, generationType NodeGenerationType) *SyscallNode {
	var imageTags []string
	now := time.Now()
	if len(imageTag) != 0 {
		imageTags = append(imageTags, imageTag)
	}
	return &SyscallNode{
		Syscall:        syscall,
		GenerationType: generationType,
		ImageTags:      imageTags,
		FirstSeen:      now,
		LastSeen:       now,
	}
}
