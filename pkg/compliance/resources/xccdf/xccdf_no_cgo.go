//go:build !libopenscap || !cgo || !linux
// +build !libopenscap !cgo !linux

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package xccdf

import (
	"context"
	"os/exec"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gocomply/scap/pkg/scap/scap_document"
)

func resolve(_ context.Context, e env.Env, id string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	xccdf := res.Xccdf

	args := []string{"xccdf", "eval", "--results", "-"}
	if xccdf.Profile != "" {
		args = append(args, "--profile", xccdf.Profile)
	}

	if xccdf.Rule != "" {
		args = append(args, "--rule", xccdf.Rule)
	}

	if xccdf.Cpe != "" {
		args = append(args, "--cpe", xccdf.Cpe+".xml")
	}

	args = append(args, res.Xccdf.Name+".xml")

	cmd := exec.Command("oscap", args...)
	cmd.Dir = e.ConfigDir()

	log.Debugf("Executing %s in %s\n", cmd.String(), cmd.Dir)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	doc, err := scap_document.ReadDocument(stdout)
	if err != nil {
		return nil, err
	}

	var instances []resources.ResolvedInstance
	for _, testResult := range doc.Benchmark.TestResult {
		for _, ruleResult := range testResult.RuleResult {
			if xccdf.Rule != "" && ruleResult.Idref != xccdf.Rule {
				continue
			}
			instances = append(instances, resources.NewResolvedInstance(
				eval.NewInstance(eval.VarMap{}, eval.FunctionMap{}, eval.RegoInputMap{
					"name":   ruleResult.Idref,
					"result": ruleResult.Result,
				}), "", ""))
		}
	}

	return resources.NewResolvedInstances(instances), nil
}
