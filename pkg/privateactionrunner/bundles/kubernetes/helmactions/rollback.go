// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package com_datadoghq_kubernetes_helmactions

import (
	"context"
	"fmt"

	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
	helmactionsimpl "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/impl"
	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	batchv1 "k8s.io/api/batch/v1"
)

type HelmRollbackHandler struct {
	ha helmactions.Component
}

func NewRollbackHandler(ha helmactions.Component) types.Action {
	return &HelmRollbackHandler{
		ha: ha,
	}
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

	executor := helmactionsimpl.NewRollbackExecutor(client)
	job, err := executor.Run(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("create helm rollback job in %s: %w", in.JobNamespace, err)
	}

	log.Infof("[HelmActions] Created rollback job %s/%s for release %s/%s (revision=%d)",
		job.Namespace, job.Name, in.ReleaseNamespace, in.Release, in.Revision)

	// todo(dp): error core processing to indicete that HA is down?
	rh.ha.OnRollback(job)

	return &HelmRollbackOutputs{
		Job: job,
	}, nil
}
