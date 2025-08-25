// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// DNSNode is used to store a DNS node
type DNSNode struct {
	NodeBase
	MatchedRules   []*model.MatchedRule
	GenerationType NodeGenerationType
	Requests       []model.DNSEvent
}

// NewDNSNode returns a new DNSNode instance
func NewDNSNode(event *model.DNSEvent, evt *model.Event, rules []*model.MatchedRule, generationType NodeGenerationType, imageTag string) *DNSNode {
	node := &DNSNode{
		MatchedRules:   rules,
		GenerationType: generationType,
		Requests:       []model.DNSEvent{*event},
	}
	node.NodeBase = NewNodeBase()
	node.AppendImageTag(imageTag, evt.ResolveEventTime())
	return node
}

func dnsFilterSubdomains(name string, maxDepth int) string {
	tab := strings.Split(name, ".")
	if len(tab) < maxDepth {
		return name
	}
	result := ""
	for i := 0; i < maxDepth; i++ {
		if result != "" {
			result = "." + result
		}
		result = tab[len(tab)-i-1] + result
	}
	return result
}

func (dn *DNSNode) evictImageTag(imageTag string, DNSNames *utils.StringKeys) bool {
	IsNodeEmpty := dn.EvictImageTag(imageTag)
	if IsNodeEmpty {
		return true
	}
	// reconstruct the list of all DNS requests
	if len(dn.Requests) > 0 {
		DNSNames.Insert(dn.Requests[0].Question.Name)
	}
	return false
}
