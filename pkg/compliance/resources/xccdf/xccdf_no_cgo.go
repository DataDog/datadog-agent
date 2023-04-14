// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !libopenscap || !cgo || !linux
// +build !libopenscap !cgo !linux

package xccdf

import (
	"context"
	"os"
	"os/exec"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	"github.com/DataDog/datadog-agent/pkg/config"
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

var mu sync.Mutex

func resolve(_ context.Context, e env.Env, id string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	// Only execute a single instance of oscap tool at a time, so it we don't consume too much memory.
	mu.Lock()
	defer mu.Unlock()

	if config.IsContainerized() {
		hostRoot := os.Getenv("HOST_ROOT")
		if hostRoot == "" {
			hostRoot = "/host"
		}

		os.Setenv("OSCAP_PROBE_ROOT", hostRoot)
		defer os.Unsetenv("OSCAP_PROBE_ROOT")
	}

	xccdf := res.Xccdf

	args := []string{"xccdf", "eval", "--skip-valid", "--results", "-"}
	if xccdf.Profile != "" {
		args = append(args, "--profile", xccdf.Profile)
	}

	var rules []string
	if res.Xccdf.Rule != "" {
		rules = []string{res.Xccdf.Rule}
	} else {
		rules = res.Xccdf.Rules
	}

	for _, rule := range rules {
		args = append(args, "--rule", rule)
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
		_ = cmd.Wait()
		return nil, err
	}

	// Ignore exit code, since oscap returns 2 on a failing result.
	_ = cmd.Wait()

	var instances []resources.ResolvedInstance
	for _, testResult := range doc.Benchmark.TestResult {
		for _, ruleResult := range testResult.RuleResult {
			ruleRef := ruleResult.Idref
			var result string
			switch ruleResult.Result {
			case XCCDF_RESULT_PASS:
				result = "passed"
			case XCCDF_RESULT_FAIL:
				result = "failing"
			case XCCDF_RESULT_ERROR, XCCDF_RESULT_UNKNOWN:
				result = "error"
			case XCCDF_RESULT_NOT_APPLICABLE:
			case XCCDF_RESULT_NOT_CHECKED, XCCDF_RESULT_NOT_SELECTED:
			}
			if result != "" {
				instances = append(instances, resources.NewResolvedInstance(
					eval.NewInstance(eval.VarMap{}, eval.FunctionMap{}, eval.RegoInputMap{
						"name":   e.Hostname(),
						"result": result,
						"rule":   ruleRef,
					}), e.Hostname(), "host"))
			}
		}
	}

	return resources.NewResolvedInstances(instances), nil
}
