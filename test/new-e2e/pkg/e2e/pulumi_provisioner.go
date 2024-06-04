// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	pulumiProvisionerDefaultID = "pulumi"
)

// PulumiEnvRunFunc is a function that runs a Pulumi program with a given environment.
type PulumiEnvRunFunc[Env any] func(ctx *pulumi.Context, env *Env) error

// PulumiProvisioner is a provisioner based on Pulumi with binding to an environment.
type PulumiProvisioner[Env any] struct {
	id           string
	runFunc      PulumiEnvRunFunc[Env]
	configMap    runner.ConfigMap
	diagnoseFunc func(ctx context.Context, stackName string) (string, error)
}

var (
	_ TypedProvisioner[any] = &PulumiProvisioner[any]{}
	_ UntypedProvisioner    = &PulumiProvisioner[any]{}
)

// NewTypedPulumiProvisioner returns a new PulumiProvisioner.
func NewTypedPulumiProvisioner[Env any](id string, runFunc PulumiEnvRunFunc[Env], configMap runner.ConfigMap) *PulumiProvisioner[Env] {
	if id == "" {
		id = pulumiProvisionerDefaultID
	}

	return &PulumiProvisioner[Env]{
		id:        id,
		runFunc:   runFunc,
		configMap: configMap,
	}
}

// NewUntypedPulumiProvisioner returns a new PulumiProvisioner without env binding.
func NewUntypedPulumiProvisioner(id string, runFunc pulumi.RunFunc, configMap runner.ConfigMap) *PulumiProvisioner[any] {
	return NewTypedPulumiProvisioner(id, func(ctx *pulumi.Context, _ *any) error {
		return runFunc(ctx)
	}, configMap)
}

// ID returns the ID of the provisioner.
func (pp *PulumiProvisioner[Env]) ID() string {
	return pp.id
}

// Provision runs the Pulumi program and returns the raw resources.
func (pp *PulumiProvisioner[Env]) Provision(ctx context.Context, stackName string, logger io.Writer) (RawResources, error) {
	return pp.ProvisionEnv(ctx, stackName, logger, nil)
}

// ProvisionEnv runs the Pulumi program with a given environment and returns the raw resources.
func (pp *PulumiProvisioner[Env]) ProvisionEnv(ctx context.Context, stackName string, logger io.Writer, env *Env) (RawResources, error) {
	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(
		ctx,
		stackName,
		pp.configMap,
		func(ctx *pulumi.Context) error {
			return pp.runFunc(ctx, env)
		},
		false,
		logger,
		nil,
	)
	if err != nil {
		return nil, err
	}

	resources := make(RawResources, len(stackOutput.Outputs))
	for key, value := range stackOutput.Outputs {
		// Skipping legacy outputs that are not maps
		if reflect.TypeOf(value.Value).Kind() != reflect.Map {
			continue
		}

		// Unfortunately we don't have access to Pulumi raw data
		marshalled, err := json.Marshal(value.Value)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal output key: %s, err: %w", key, err)
		}

		resources[key] = marshalled
	}

	return resources, nil
}

// Diagnose runs the diagnose function if it is set diagnoseFunc
func (pp *PulumiProvisioner[Env]) Diagnose(ctx context.Context, stackName string) (string, error) {
	if pp.diagnoseFunc != nil {
		return pp.diagnoseFunc(ctx, stackName)
	}
	return "", nil
}

// SetDiagnoseFunc sets the diagnose function.
func (pp *PulumiProvisioner[Env]) SetDiagnoseFunc(diagnoseFunc func(ctx context.Context, stackName string) (string, error)) {
	pp.diagnoseFunc = diagnoseFunc
}

// Destroy deletes the Pulumi stack.
func (pp *PulumiProvisioner[Env]) Destroy(ctx context.Context, stackName string, logger io.Writer) error {
	return infra.GetStackManager().DeleteStack(ctx, stackName, logger)
}
