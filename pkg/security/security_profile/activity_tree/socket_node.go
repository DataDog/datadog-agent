// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package activity_tree

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// BindNode is used to store a bind node
type BindNode struct {
	MatchedRules []*model.MatchedRule

	GenerationType NodeGenerationType
	Port           uint16
	IP             string
}

// SocketNode is used to store a Socket node and associated events
type SocketNode struct {
	Family         string
	GenerationType NodeGenerationType
	Bind           []*BindNode
}

// InsertBindEvent inserts a bind even inside a socket node
func (n *SocketNode) InsertBindEvent(evt *model.BindEvent, generationType NodeGenerationType, rules []*model.MatchedRule, shadowInsertion bool) bool {
	evtIP := evt.Addr.IPNet.IP.String()

	for _, n := range n.Bind {
		if evt.Addr.Port == n.Port && evtIP == n.IP {
			if !shadowInsertion {
				n.MatchedRules = model.AppendMatchedRule(n.MatchedRules, rules)
			}
			return false
		}
	}

	if !shadowInsertion {
		// insert bind event now
		n.Bind = append(n.Bind, &BindNode{
			MatchedRules:   rules,
			GenerationType: generationType,
			Port:           evt.Addr.Port,
			IP:             evtIP,
		})
	}
	return true
}

// NewSocketNode returns a new SocketNode instance
func NewSocketNode(family string, generationType NodeGenerationType) *SocketNode {
	return &SocketNode{
		Family:         family,
		GenerationType: generationType,
	}
}
