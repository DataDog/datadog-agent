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

// BindNode is used to store a bind node
type BindNode struct {
	NodeBase

	MatchedRules []*model.MatchedRule

	GenerationType NodeGenerationType
	Port           uint16
	IP             string
	Protocol       uint16
}

// SocketNode is used to store a Socket node and associated events
type SocketNode struct {
	NodeBase
	Family         string
	GenerationType NodeGenerationType
	Bind           []*BindNode
}

// Matches returns true if BindNodes matches
func (bn *BindNode) Matches(toMatch *BindNode) bool {
	return bn.Port == toMatch.Port && bn.IP == toMatch.IP && bn.Protocol == toMatch.Protocol
}

// Matches returns true if SocketNodes matches
func (sn *SocketNode) Matches(toMatch *SocketNode) bool {
	return sn.Family == toMatch.Family
}

func (sn *SocketNode) evictImageTag(imageTag string) bool {
	newBind := []*BindNode{}
	for _, bind := range sn.Bind {
		if shouldRemoveNode := bind.EvictImageTag(imageTag); !shouldRemoveNode {
			newBind = append(newBind, bind)
		}
	}
	if len(newBind) == 0 {
		return true
	}
	sn.Bind = newBind
	return false
}

// InsertBindEvent inserts a bind even inside a socket node
func (sn *SocketNode) InsertBindEvent(evt *model.BindEvent, event *model.Event, imageTag string, generationType NodeGenerationType, rules []*model.MatchedRule, dryRun bool) bool {
	evtIP := evt.Addr.IPNet.IP.String()

	for _, n := range sn.Bind {
		if evt.Addr.Port == n.Port && evtIP == n.IP && evt.Protocol == n.Protocol {
			if !dryRun {
				n.MatchedRules = model.AppendMatchedRule(n.MatchedRules, rules)
			}
			if imageTag == "" || n.HasImageTag(imageTag) {
				return false
			}
			n.AppendImageTag(imageTag, event.ResolveEventTime())
			return false
		}
	}

	if !dryRun {
		// insert bind event now
		node := &BindNode{
			MatchedRules:   rules,
			GenerationType: generationType,
			Port:           evt.Addr.Port,
			IP:             evtIP,
			Protocol:       evt.Protocol,
		}
		node.NodeBase = NewNodeBase()

		node.AppendImageTag(imageTag, event.ResolveEventTime())
		sn.Bind = append(sn.Bind, node)
	}
	return true
}

// NewSocketNode returns a new SocketNode instance
func NewSocketNode(family string, generationType NodeGenerationType) *SocketNode {
	node := &SocketNode{
		Family:         family,
		GenerationType: generationType,
	}
	node.NodeBase = NewNodeBase()
	return node
}
