// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package healthcheck

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/executor"
)

// synthesizeAllowlist substitutes explicit local policy for normally signed remote_action inputs.
func synthesizeAllowlist(cfg integration.RemediationConfig) (*pb.RemediationAllowlist, error) {
	policy := &pb.RemediationAllowlist{
		AllowedPaths: []string{}, AllowedCommands: []string{},
		AllowedServices: make(map[string]*pb.RemediationServiceActions),
	}
	for _, path := range cfg.AllowedPaths {
		if !literalPolicyEntry(path) {
			return nil, errors.New("allowed paths must be non-empty and contain no wildcards")
		}
		policy.AllowedPaths = append(policy.AllowedPaths, path)
	}
	for service, actions := range cfg.AllowedServices {
		if !literalPolicyEntry(service) {
			return nil, errors.New("allowed services must be non-empty and contain no wildcards")
		}
		for _, action := range actions {
			if !literalPolicyEntry(action) {
				return nil, errors.New("allowed service actions must be non-empty and contain no wildcards")
			}
		}
		policy.AllowedServices[service] = &pb.RemediationServiceActions{Actions: slices.Clone(actions)}
	}
	if len(cfg.Steps) == 0 || len(cfg.Steps) > 32 {
		return nil, errors.New("remediation requires between one and 32 steps")
	}
	for i, step := range cfg.Steps {
		if strings.TrimSpace(step.Command) == "" {
			return nil, errors.New("remediation commands must not be empty")
		}
		program, err := syntax.NewParser().Parse(strings.NewReader(step.Command), "")
		if err != nil {
			return nil, fmt.Errorf("parse remediation step %d: %w", i+1, err)
		}
		syntax.Walk(program, func(node syntax.Node) bool {
			if err != nil {
				return false
			}
			call, ok := node.(*syntax.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			command := call.Args[0].Lit()
			if !literalPolicyEntry(command) || strings.ContainsAny(command, "/\\:$ \t\r\n") || command == "sudo" {
				err = errors.New("remediation requires literal command names without wildcards or escalation")
				return false
			}
			policy.AllowedCommands = append(policy.AllowedCommands, "rshell:"+command)
			return true
		})
		if err != nil {
			return nil, err
		}
	}
	slices.Sort(policy.AllowedCommands)
	policy.AllowedCommands = slices.Compact(policy.AllowedCommands)
	return policy, nil
}

func literalPolicyEntry(value string) bool {
	return strings.TrimSpace(value) != "" && !strings.ContainsAny(value, "*?[]{}\x00")
}
