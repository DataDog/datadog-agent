// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package test

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type runFunc[Env any] func(ctx *pulumi.Context, env *Env) error

// StackDefinition contains a Pulumi stack definition
type StackDefinition[Env any] struct {
	envFactory runFunc[Env]
	configMap  runner.ConfigMap
}

// NewStackDef creates a custom definition
func NewStackDef[Env any](envFactory runFunc[Env], configMap runner.ConfigMap) *StackDefinition[Env] {
	return &StackDefinition[Env]{envFactory: envFactory, configMap: configMap}
}

// EnvFactoryStackDef creates a custom stack definition
func EnvFactoryStackDef[Env any](envFactory runFunc[Env]) *StackDefinition[Env] {
	return NewStackDef(envFactory, runner.ConfigMap{})
}
