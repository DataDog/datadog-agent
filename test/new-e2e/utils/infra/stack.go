// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package infra

import (
	"context"
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Stack struct {
	projectName string
	stackName   string
	stack       auto.Stack
}

func NewStack(ctx context.Context, projectName, stackName string, config auto.ConfigMap, deployFunc pulumi.RunFunc) (*Stack, error) {
	stack, err := auto.UpsertStackInlineSource(ctx, stackName, projectName, deployFunc)
	if err != nil {
		return nil, err
	}

	err = stack.SetAllConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	return &Stack{
		projectName: projectName,
		stackName:   stackName,
		stack:       stack,
	}, nil
}

func (st *Stack) Up(ctx context.Context) (*auto.UpResult, error) {
	_, err := st.stack.Refresh(ctx)
	if err != nil {
		return nil, err
	}

	result, err := st.stack.Up(ctx, optup.ProgressStreams(os.Stdout))
	if err != nil {
		return nil, err
	}

	return &result, err
}

func (st *Stack) Down(ctx context.Context) error {
	_, err := st.stack.Refresh(ctx)
	if err != nil {
		return err
	}

	_, err = st.stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
	return err
}
