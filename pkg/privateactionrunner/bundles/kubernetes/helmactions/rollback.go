// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package com_datadoghq_kubernetes_helmactions

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/helmactions"
	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	batchv1 "k8s.io/api/batch/v1"
)

type HelmRollbackHandler struct{}

func NewRollbackHandler() types.Action {
	return &HelmRollbackHandler{}
}

type HelmRollbackOutputs struct {
	Job *batchv1.Job
}

// Run returns any which is the serialized and send back to DD backend
func (rh *HelmRollbackHandler) Run(ctx context.Context, task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	in, err := types.ExtractInputs[helmactions.RollbackInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	executor := helmactions.NewRollbackExecutor(client)
	job, err := executor.Run(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("helm rollback job: %w", err)
	}

	// todo(dp): remove after debug is complete
	fmt.Printf("Created helm rollback job %s/%s for release %s:%s (revision=%d)\n",
		job.Namespace, job.Name, in.ReleaseNamespace, in.Release, 0)

	return &HelmRollbackOutputs{
		Job: job,
	}, nil
}
