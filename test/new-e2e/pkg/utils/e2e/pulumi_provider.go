// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"context"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/infra"
)

var _ InfraProvider[int] = (*PulumiProvider[int])(nil)

// PulumiProvider leverages pulumi to create and manage testing infrastructure
type PulumiProvider[Env any] struct {
	envFactory func(ctx *pulumi.Context) (*Env, error)
	configMap  runner.ConfigMap

	stackManager *infra.StackManager
}

// NewPulumiProvider returns a new PulumiProvider
func NewPulumiProvider[Env any](envFactory func(ctx *pulumi.Context) (*Env, error), configMap runner.ConfigMap) *PulumiProvider[Env] {
	return &PulumiProvider[Env]{
		envFactory:   envFactory,
		configMap:    configMap,
		stackManager: infra.GetStackManager(),
	}
}

// ProvisionInfraAndInitializeEnv uses a pulumi stack manager to initialize a pulumi stack & pass the resulting UpResult to
// any clients in the environment which implement the pulumiStackInitializer interface
func (ps *PulumiProvider[Env]) ProvisionInfraAndInitializeEnv(ctx context.Context, t *testing.T, name string, failOnMissing bool) (*Env, error) {
	var env *Env

	deployFunc := func(ctx *pulumi.Context) error {
		var err error
		env, err = ps.envFactory(ctx)
		return err
	}
	_, stackResult, err := ps.stackManager.GetStackNoDeleteOnFailure(ctx, name, ps.configMap, deployFunc, failOnMissing)
	if err != nil {
		return nil, err
	}

	err = client.CallStackInitializers(t, env, stackResult)
	return env, err
}

// DeleteInfra deletes the pulumi stack
func (ps *PulumiProvider[Env]) DeleteInfra(ctx context.Context, name string) error {
	return ps.stackManager.DeleteStack(ctx, name)
}
