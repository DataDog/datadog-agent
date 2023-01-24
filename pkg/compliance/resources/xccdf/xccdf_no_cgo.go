// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !libopenscap || !cgo || !linux
// +build !libopenscap !cgo !linux

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

const (
	XCCDF_RESULT_PASS           = "pass"
	XCCDF_RESULT_FAIL           = "fail"
	XCCDF_RESULT_ERROR          = "error"
	XCCDF_RESULT_UNKNOWN        = "unknown"
	XCCDF_RESULT_NOT_APPLICABLE = "notapplicable"
	XCCDF_RESULT_NOT_CHECKED    = "notchecked"
	XCCDF_RESULT_NOT_SELECTED   = "notselected"
)

func resolve(_ context.Context, e env.Env, id string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	xccdf := res.Xccdf

	args := []string{"xccdf", "eval", "--skip-valid", "--results", "-"}
	if xccdf.Profile != "" {
		args = append(args, "--profile", xccdf.Profile)
	}

	if xccdf.Rule != "" {
		args = append(args, "--rule", xccdf.Rule)
	}

	args = append(args, res.Xccdf.Name)
	cmd := exec.Command("/opt/datadog-agent/embedded/bin/oscap", args...)
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
			var result string
			switch ruleResult.Result {
			case XCCDF_RESULT_PASS:
				result = "passed"
			case XCCDF_RESULT_FAIL:
				result = "failing"
			case XCCDF_RESULT_ERROR, XCCDF_RESULT_UNKNOWN:
				result = "error"
			case XCCDF_RESULT_NOT_APPLICABLE, XCCDF_RESULT_NOT_CHECKED, XCCDF_RESULT_NOT_SELECTED:
			}
			if result != "" {
				instances = append(instances, resources.NewResolvedInstance(
					eval.NewInstance(eval.VarMap{}, eval.FunctionMap{}, eval.RegoInputMap{
						"name":   e.Hostname(),
						"result": result,
					}), ruleResult.Idref, ""))
			}
		}
	}

	return resources.NewResolvedInstances(instances), nil
}
