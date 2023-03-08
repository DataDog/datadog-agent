// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package dump

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Profile holds the list of rules generated from an activity dump
type Profile struct {
	Name     string
	Selector string
	Rules    []ProfileRule
}

// ProfileRule contains the data required to generate a rule
type ProfileRule struct {
	ID         string
	Expression string
}

// ProfileTemplate is the template used to generate profiles
var ProfileTemplate = `---
name: {{ .Name }}
selector:
  - {{ .Selector }}

rules:{{ range .Rules }}
  - id: {{ .ID }}
    expression: {{ .Expression }}
{{ end }}
`

// NewProfileRule returns a new ProfileRule
func NewProfileRule(expression string, ruleIDPrefix string) ProfileRule {
	return ProfileRule{
		ID:         ruleIDPrefix + "_" + utils.RandString(5),
		Expression: expression,
	}
}

func (ad *ActivityDump) generateDNSRule(dns *DNSNode, activityNode *ProcessActivityNode, ancestors []*ProcessActivityNode, ruleIDPrefix string) []ProfileRule {
	var rules []ProfileRule

	if dns != nil {
		for _, req := range dns.Requests {
			rule := NewProfileRule(fmt.Sprintf(
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

func (ad *ActivityDump) generateBindRule(sock *SocketNode, activityNode *ProcessActivityNode,
	ancestors []*ProcessActivityNode, ruleIDPrefix string) []ProfileRule {
	var rules []ProfileRule

	if sock != nil {
		var socketRules []ProfileRule
		if len(sock.Bind) > 0 {
			for _, bindNode := range sock.Bind {
				socketRules = append(socketRules, NewProfileRule(fmt.Sprintf(
					"bind.addr.family == %s && bind.addr.ip in %s/32 && bind.addr.port == %d",
					sock.Family, bindNode.IP, bindNode.Port),
					ruleIDPrefix,
				))
			}
		} else {
			socketRules = []ProfileRule{NewProfileRule(fmt.Sprintf("bind.addr.family == %s", sock.Family),
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

func (ad *ActivityDump) generateFIMRules(file *FileActivityNode, activityNode *ProcessActivityNode, ancestors []*ProcessActivityNode, ruleIDPrefix string) []ProfileRule {
	var rules []ProfileRule

	if file.File == nil {
		return rules
	}

	if file.Open != nil {
		rule := NewProfileRule(fmt.Sprintf(
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
		childrenRules := ad.generateFIMRules(child, activityNode, ancestors, ruleIDPrefix)
		rules = append(rules, childrenRules...)
	}

	return rules
}

func (ad *ActivityDump) generateRules(node *ProcessActivityNode, ancestors []*ProcessActivityNode, ruleIDPrefix string) []ProfileRule {
	var rules []ProfileRule

	// add exec rule
	rule := NewProfileRule(fmt.Sprintf(
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
		fimRules := ad.generateFIMRules(file, node, ancestors, ruleIDPrefix)
		rules = append(rules, fimRules...)
	}

	// add DNS rules
	for _, dns := range node.DNSNames {
		dnsRules := ad.generateDNSRule(dns, node, ancestors, ruleIDPrefix)
		rules = append(rules, dnsRules...)
	}

	// add Bind rules
	for _, sock := range node.Sockets {
		bindRules := ad.generateBindRule(sock, node, ancestors, ruleIDPrefix)
		rules = append(rules, bindRules...)
	}

	// add children rules recursively
	newAncestors := append([]*ProcessActivityNode{node}, ancestors...)
	for _, child := range node.Children {
		childrenRules := ad.generateRules(child, newAncestors, ruleIDPrefix)
		rules = append(rules, childrenRules...)
	}

	return rules
}

// GenerateProfileData generates a Profile from the activity dump
func (ad *ActivityDump) GenerateProfileData() Profile {
	ad.Lock()
	defer ad.Unlock()

	p := Profile{
		Name: "profile_" + utils.RandString(5),
	}

	// generate selector
	if len(ad.Metadata.Comm) > 0 {
		p.Selector = fmt.Sprintf("process.comm = \"%s\"", ad.Metadata.Comm)
	}

	// Add rules
	for _, node := range ad.ProcessActivityTree {
		rules := ad.generateRules(node, nil, p.Name)
		p.Rules = append(p.Rules, rules...)
	}

	return p
}

// EncodeSecL encodes an activity dump in the Profile format
func (ad *ActivityDump) EncodeSecL() (*bytes.Buffer, error) {
	t := template.Must(template.New("tmpl").Parse(ProfileTemplate))
	raw := bytes.NewBuffer(nil)
	if err := t.Execute(raw, ad.GenerateProfileData()); err != nil {
		return nil, fmt.Errorf("couldn't generate profile: %w", err)
	}
	return raw, nil
}
