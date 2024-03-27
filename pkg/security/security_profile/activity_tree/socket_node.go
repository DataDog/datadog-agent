// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"golang.org/x/exp/slices"
)

// BindNode is used to store a bind node
type BindNode struct {
	MatchedRules []*model.MatchedRule
	ImageTags    []string

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

// Matches returns true if BindNodes matches
func (bn *BindNode) Matches(toMatch *BindNode) bool {
	return bn.Port == toMatch.Port && bn.IP == toMatch.IP
}

// Matches returns true if BindNodes matches
func (sn *SocketNode) Matches(toMatch *SocketNode) bool {
	return sn.Family == toMatch.Family
}

func (bn *BindNode) appendImageTag(imageTag string) {
	bn.ImageTags, _ = AppendIfNotPresent(bn.ImageTags, imageTag)
}

func (sn *SocketNode) appendImageTag(imageTag string) {
	for _, bn := range sn.Bind {
		bn.appendImageTag(imageTag)
	}
}

func (bn *BindNode) evictImageTag(imageTag string) bool {
	imageTags, removed := removeImageTagFromList(bn.ImageTags, imageTag)
	if !removed {
		return false
	}
	if len(imageTags) == 0 {
		return true
	}
	bn.ImageTags = imageTags
	return false
}

func (sn *SocketNode) evictImageTag(imageTag string) bool {
	newBind := []*BindNode{}
	for _, bind := range sn.Bind {
		if shouldRemoveNode := bind.evictImageTag(imageTag); !shouldRemoveNode {
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
func (sn *SocketNode) InsertBindEvent(evt *model.BindEvent, imageTag string, generationType NodeGenerationType, rules []*model.MatchedRule, dryRun bool) bool {
	evtIP := evt.Addr.IPNet.IP.String()

	for _, n := range sn.Bind {
		if evt.Addr.Port == n.Port && evtIP == n.IP {
			if !dryRun {
				n.MatchedRules = model.AppendMatchedRule(n.MatchedRules, rules)
			}
			if imageTag == "" || slices.Contains(n.ImageTags, imageTag) {
				return false
			}
			n.ImageTags = append(n.ImageTags, imageTag)
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
		}
		if imageTag != "" {
			node.ImageTags = []string{imageTag}
		}
		sn.Bind = append(sn.Bind, node)
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
