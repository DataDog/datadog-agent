// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// IMDSInfo is the slimmed-down representation of a model.IMDSEvent kept in the
// activity tree.
type IMDSInfo struct {
	Type          string
	CloudProvider string
	URL           string
	Host          string
	UserAgent     string
	Server        string
	AWS           AWSIMDSInfo
}

// AWSIMDSInfo holds the AWS-specific fields kept from an IMDS event.
type AWSIMDSInfo struct {
	IsIMDSv2            bool
	SecurityCredentials AWSSecurityCredentialsInfo
}

// AWSSecurityCredentialsInfo holds the AWS security credential fields kept from an IMDS event.
type AWSSecurityCredentialsInfo struct {
	Code          string
	Type          string
	AccessKeyID   string
	LastUpdated   string
	ExpirationRaw string
}

// newIMDSInfo builds a slim IMDSInfo from a model.IMDSEvent.
func newIMDSInfo(e *model.IMDSEvent) IMDSInfo {
	return IMDSInfo{
		Type:          e.Type,
		CloudProvider: e.CloudProvider,
		URL:           e.URL,
		Host:          e.Host,
		UserAgent:     e.UserAgent,
		Server:        e.Server,
		AWS: AWSIMDSInfo{
			IsIMDSv2: e.AWS.IsIMDSv2,
			SecurityCredentials: AWSSecurityCredentialsInfo{
				Code:          e.AWS.SecurityCredentials.Code,
				Type:          e.AWS.SecurityCredentials.Type,
				AccessKeyID:   e.AWS.SecurityCredentials.AccessKeyID,
				LastUpdated:   e.AWS.SecurityCredentials.LastUpdated,
				ExpirationRaw: e.AWS.SecurityCredentials.ExpirationRaw,
			},
		},
	}
}

// IMDSNode is used to store a IMDS node
type IMDSNode struct {
	NodeBase
	MatchedRules   []*model.MatchedRule
	GenerationType NodeGenerationType
	Event          IMDSInfo
}

// size approximates this node's heap footprint
func (in *IMDSNode) size() int64 {
	s := int64(unsafe.Sizeof(*in))
	s += seenBytes(in.NodeBase)
	s += int64(len(in.Event.Type))
	s += int64(len(in.Event.CloudProvider))
	s += int64(len(in.Event.URL))
	s += int64(len(in.Event.Host))
	s += int64(len(in.Event.UserAgent))
	s += int64(len(in.Event.Server))
	s += int64(len(in.Event.AWS.SecurityCredentials.Code))
	s += int64(len(in.Event.AWS.SecurityCredentials.Type))
	s += int64(len(in.Event.AWS.SecurityCredentials.AccessKeyID))
	s += int64(len(in.Event.AWS.SecurityCredentials.LastUpdated))
	s += int64(len(in.Event.AWS.SecurityCredentials.ExpirationRaw))
	s += sliceBackingBytes(cap(in.MatchedRules), unsafe.Sizeof((*model.MatchedRule)(nil)))
	return s
}

// NewIMDSNode creates a new IMDSNode instance
func NewIMDSNode(event *model.IMDSEvent, evt *model.Event, rules []*model.MatchedRule, generationType NodeGenerationType, imageTagID uint64) *IMDSNode {
	node := &IMDSNode{
		MatchedRules:   rules,
		GenerationType: generationType,
		Event:          newIMDSInfo(event),
	}
	node.NodeBase = NewNodeBase()
	node.AppendImageTagID(imageTagID, evt.ResolveEventTime())

	return node
}
