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
)

// DNSNode is used to store a DNS node
type DNSNode struct {
	MatchedRules []*model.MatchedRule

	GenerationType NodeGenerationType
	Requests       []model.DNSEvent
}

// NewDNSNode returns a new DNSNode instance
func NewDNSNode(event *model.DNSEvent, rules []*model.MatchedRule, generationType NodeGenerationType) *DNSNode {
	return &DNSNode{
		MatchedRules:   rules,
		GenerationType: generationType,
		Requests:       []model.DNSEvent{*event},
	}
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
