// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package activity_tree

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// SeclEncoding holds the list of rules generated from an activity dump
type SeclEncoding struct {
	Name     string
	Selector string
	Rules    []eval.Rule
}

// NewSeclRule returns a new ProfileRule
func NewSeclRule(expression string, ruleIDPrefix string) eval.Rule {
	return eval.Rule{
		ID:         ruleIDPrefix + "_" + utils.RandString(5),
		Expression: expression,
	}
}

func (at *ActivityTree) generateDNSRule(dns *DNSNode, activityNode *ProcessNode, ancestors []*ProcessNode, ruleIDPrefix string) []eval.Rule {
	var rules []eval.Rule

	if dns != nil {
		for _, req := range dns.Requests {
			rule := NewSeclRule(fmt.Sprintf(
				"dns.question.name == \"%s\" && dns.question.type == \"%s\"",
				req.Name,
				model.QType(req.Type).String()),
				ruleIDPrefix,
			)
			rule.Expression += fmt.Sprintf(" && process.file.path == \"%s\"", activityNode.Process.FileEvent.PathnameStr)
			for _, parent := range ancestors {
				rule.Expression += fmt.Sprintf(" && process.ancestors.file.path == \"%s\"", parent.Process.FileEvent.PathnameStr)
			}
			rules = append(rules, rule)
		}
	}

	return rules
}

func (at *ActivityTree) generateBindRule(sock *SocketNode, activityNode *ProcessNode,
	ancestors []*ProcessNode, ruleIDPrefix string) []eval.Rule {
	var rules []eval.Rule

	if sock != nil {
		var socketRules []eval.Rule
		if len(sock.Bind) > 0 {
			for _, bindNode := range sock.Bind {
				socketRules = append(socketRules, NewSeclRule(fmt.Sprintf(
					"bind.addr.family == %s && bind.addr.ip in %s/32 && bind.addr.port == %d",
					sock.Family, bindNode.IP, bindNode.Port),
					ruleIDPrefix,
				))
			}
		} else {
			socketRules = []eval.Rule{NewSeclRule(fmt.Sprintf("bind.addr.family == %s", sock.Family),
				ruleIDPrefix,
			)}
		}

		for i := range socketRules {
			socketRules[i].Expression += fmt.Sprintf(" && process.file.path == \"%s\"",
				activityNode.Process.FileEvent.PathnameStr)
			for _, parent := range ancestors {
				socketRules[i].Expression += fmt.Sprintf(" && process.ancestors.file.path == \"%s\"",
					parent.Process.FileEvent.PathnameStr)
			}
		}
		rules = append(rules, socketRules...)
	}

	return rules
}

func (at *ActivityTree) generateFIMRules(file *FileNode, activityNode *ProcessNode, ancestors []*ProcessNode, ruleIDPrefix string) []eval.Rule {
	var rules []eval.Rule

	if file.File == nil {
		return rules
	}

	if file.Open != nil {
		rule := NewSeclRule(fmt.Sprintf(
			"open.file.path == \"%s\" && open.file.in_upper_layer == %v && open.file.uid == %d && open.file.gid == %d",
			file.File.PathnameStr,
			file.File.InUpperLayer,
			file.File.UID,
			file.File.GID),
			ruleIDPrefix,
		)
		rule.Expression += fmt.Sprintf(" && process.file.path == \"%s\"", activityNode.Process.FileEvent.PathnameStr)
		for _, parent := range ancestors {
			rule.Expression += fmt.Sprintf(" && process.ancestors.file.path == \"%s\"", parent.Process.FileEvent.PathnameStr)
		}
		rules = append(rules, rule)
	}

	for _, child := range file.Children {
		childrenRules := at.generateFIMRules(child, activityNode, ancestors, ruleIDPrefix)
		rules = append(rules, childrenRules...)
	}

	return rules
}

func (at *ActivityTree) generateRules(node *ProcessNode, ancestors []*ProcessNode, ruleIDPrefix string) []eval.Rule {
	var rules []eval.Rule

	// add exec rule
	rule := NewSeclRule(fmt.Sprintf(
		"exec.file.path == \"%s\" && process.uid == %d && process.gid == %d && process.cap_effective == %d && process.cap_permitted == %d",
		node.Process.FileEvent.PathnameStr,
		node.Process.UID,
		node.Process.GID,
		node.Process.CapEffective,
		node.Process.CapPermitted),
		ruleIDPrefix,
	)
	for _, parent := range ancestors {
		rule.Expression += fmt.Sprintf(" && process.ancestors.file.path == \"%s\"", parent.Process.FileEvent.PathnameStr)
	}
	rules = append(rules, rule)

	// add FIM rules
	for _, file := range node.Files {
		fimRules := at.generateFIMRules(file, node, ancestors, ruleIDPrefix)
		rules = append(rules, fimRules...)
	}

	// add DNS rules
	for _, dns := range node.DNSNames {
		dnsRules := at.generateDNSRule(dns, node, ancestors, ruleIDPrefix)
		rules = append(rules, dnsRules...)
	}

	// add Bind rules
	for _, sock := range node.Sockets {
		bindRules := at.generateBindRule(sock, node, ancestors, ruleIDPrefix)
		rules = append(rules, bindRules...)
	}

	// add children rules recursively
	newAncestors := append([]*ProcessNode{node}, ancestors...)
	for _, child := range node.Children {
		childrenRules := at.generateRules(child, newAncestors, ruleIDPrefix)
		rules = append(rules, childrenRules...)
	}

	return rules
}

// GenerateProfileData generates a Profile from the activity dump
func (at *ActivityTree) GenerateProfileData(selector string) SeclEncoding {
	p := SeclEncoding{
		Name:     "profile_" + utils.RandString(5),
		Selector: selector,
	}

	// Add rules
	for _, node := range at.ProcessNodes {
		rules := at.generateRules(node, nil, p.Name)
		p.Rules = append(p.Rules, rules...)
	}

	return p
}
