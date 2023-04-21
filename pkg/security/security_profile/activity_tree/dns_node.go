// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package activity_tree

import "github.com/DataDog/datadog-agent/pkg/security/secl/model"

// DNSNode is used to store a DNS node
type DNSNode struct {
	MatchedRules []*model.MatchedRule

	Requests []model.DNSEvent
}

// NewDNSNode returns a new DNSNode instance
func NewDNSNode(event *model.DNSEvent, rules []*model.MatchedRule) *DNSNode {
	return &DNSNode{
		MatchedRules: rules,
		Requests:     []model.DNSEvent{*event},
	}
}
