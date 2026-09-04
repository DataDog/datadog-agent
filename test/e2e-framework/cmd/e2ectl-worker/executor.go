// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/workerclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
)

// Executor is the type-erased form of a scenario: the registry stores
// Provision/Destroy closures that captured the environment type once.
// New scenarios never touch this machinery — they use fromTyped.
type Executor struct {
	Provision func(ctx *standalone.Context, stackName, snapshotPath string) error
	Destroy   func(ctx *standalone.Context, stackName string) error
}

// Builder turns the raw driver-owned config section (YAML) into an Executor.
// This is the whole registration API: a scenario registers its base name
// and one builder that strict-decodes its params and wraps its run function.
type Builder func(params string) (Executor, error)

// scenarios is the worker-side registry (the mirror of the core's driver
// registry — see the extensibility plan, T4). Adding a Pulumi environment:
// write the scenario run function, register a builder here, done.
var scenarios = map[string]Builder{}

func registerScenario(base string, b Builder) {
	if _, dup := scenarios[base]; dup {
		panic(fmt.Sprintf("scenario %q registered twice", base))
	}
	scenarios[base] = b
}

// fromTyped adapts a typed provisioner into the type-erased Executor:
// provision writes the snapshot (with the stack name as metadata — the
// destroy side and the core can read it back from the single source of
// truth), destroy tears the stack down with the same provisioner.
func fromTyped[Env any](p provisioner.TypedProvisioner[Env]) Executor {
	return Executor{
		Provision: func(ctx *standalone.Context, stackName, snapshotPath string) error {
			_, resources, err := standalone.ProvisionE[Env](ctx, stackName, p)
			if err != nil {
				return err
			}
			return provisioner.WriteSnapshotFile(snapshotPath, resources, map[string]any{
				"source": "e2ectl-worker",
				"stack":  stackName,
			})
		},
		Destroy: func(ctx *standalone.Context, stackName string) error {
			return standalone.Destroy(ctx, stackName, p)
		},
	}
}

// runJob is the generic, forever-static engine: lookup by base, strict-decode
// params, provision or destroy.
func runJob(j workerclient.Job) error {
	build, ok := scenarios[j.Base]
	if !ok {
		known := make([]string, 0, len(scenarios))
		for b := range scenarios {
			known = append(known, b)
		}
		return fmt.Errorf("no scenario registered for base %q (registered: %v)", j.Base, known)
	}
	exec, err := build(j.Params)
	if err != nil {
		return fmt.Errorf("decoding scenario %q params: %w", j.Base, err)
	}
	ctx := standalone.NewContext(j.EnvDir)
	snapshotPath := j.EnvDir + "/snapshot.json"
	switch j.Action {
	case workerclient.ActionProvision:
		return exec.Provision(ctx, j.StackName, snapshotPath)
	case workerclient.ActionDestroy:
		return exec.Destroy(ctx, j.StackName)
	default:
		return fmt.Errorf("unknown action %q (supported: %s, %s)", j.Action, workerclient.ActionProvision, workerclient.ActionDestroy)
	}
}

var _ = os.Stderr
