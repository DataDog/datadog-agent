// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

// SyscallNode is used to store a syscall node
type SyscallNode struct {
	ImageTags      []string
	GenerationType NodeGenerationType

	Syscall int
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

// NewSyscallNode returns a new SyscallNode instance
func NewSyscallNode(syscall int, imageTag string, generationType NodeGenerationType) *SyscallNode {
	var imageTags []string
	if len(imageTag) != 0 {
		imageTags = append(imageTags, imageTag)
	}
	return &SyscallNode{
		Syscall:        syscall,
		GenerationType: generationType,
		ImageTags:      imageTags,
	}
}
