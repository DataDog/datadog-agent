// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

func lookupOrErr(name string) (Runnable, error) {
	r, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown scenario %q", name)
	}
	return r, nil
}

// Create provisions the named scenario into stack with the given config.
func Create(ctx common.Context, scenarioName, stack string, cfg map[string]string) error {
	r, err := lookupOrErr(scenarioName)
	if err != nil {
		return err
	}
	return r.Create(ctx, stack, cfg)
}

// RunAction runs a named action on a running stack.
func RunAction(ctx common.Context, scenarioName, stack, action string, cfg map[string]string) error {
	r, err := lookupOrErr(scenarioName)
	if err != nil {
		return err
	}
	return r.RunAction(ctx, stack, action, cfg)
}

// Destroy tears down a running stack.
func Destroy(ctx common.Context, scenarioName, stack string) error {
	r, err := lookupOrErr(scenarioName)
	if err != nil {
		return err
	}
	return r.Destroy(ctx, stack)
}
