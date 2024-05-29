// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// IMDSNode is used to store a IMDS node
type IMDSNode struct {
	MatchedRules   []*model.MatchedRule
	ImageTags      []string
	GenerationType NodeGenerationType

	Event model.IMDSEvent
}

// NewIMDSNode returns a new IMDSNode instance
func NewIMDSNode(event *model.IMDSEvent, rules []*model.MatchedRule, generationType NodeGenerationType, imageTag string) *IMDSNode {
	node := &IMDSNode{
		MatchedRules:   rules,
		GenerationType: generationType,
		Event:          *event,
	}
	if imageTag != "" {
		node.ImageTags = []string{imageTag}
	}
	return node
}

func (imds *IMDSNode) appendImageTag(imageTag string) {
	imds.ImageTags, _ = AppendIfNotPresent(imds.ImageTags, imageTag)
}

func (imds *IMDSNode) evictImageTag(imageTag string) bool {
	imageTags, removed := removeImageTagFromList(imds.ImageTags, imageTag)
	if removed {
		if len(imageTags) == 0 {
			return true
		}
		imds.ImageTags = imageTags
	}
	return false
}
