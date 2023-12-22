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

type PulumiEnvRunFunc[Env any] func(ctx *pulumi.Context, env *Env) error

type PulumiProvisioner[Env any] struct {
	id        string
	runFunc   PulumiEnvRunFunc[Env]
	configMap runner.ConfigMap
}

var (
	_ TypedProvisioner[any] = &PulumiProvisioner[any]{}
	_ UntypedProvisioner    = &PulumiProvisioner[any]{}
)

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

func NewUntypedPulumiProvisioner(id string, runFunc pulumi.RunFunc, configMap runner.ConfigMap) *PulumiProvisioner[any] {
	return NewTypedPulumiProvisioner(id, func(ctx *pulumi.Context, _ *any) error {
		return runFunc(ctx)
	}, configMap)
}

func (pp *PulumiProvisioner[Env]) ID() string {
	return pp.id
}

func (pp *PulumiProvisioner[Env]) Provision(stackName string, ctx context.Context, logger io.Writer) (RawResources, error) {
	return pp.ProvisionEnv(stackName, ctx, logger, nil)
}

func (pp *PulumiProvisioner[Env]) ProvisionEnv(stackName string, ctx context.Context, logger io.Writer, env *Env) (RawResources, error) {
	_, stackOutput, err := infra.GetStackManager().GetStackNoDeleteOnFailure(
		ctx,
		stackName,
		pp.configMap,
		func(ctx *pulumi.Context) error {
			return pp.runFunc(ctx, env)
		},
		false,
		logger,
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

func (pp *PulumiProvisioner[Env]) Destroy(stackName string, ctx context.Context, logger io.Writer) error {
	return infra.GetStackManager().DeleteStack(ctx, stackName, logger)
}
