// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runners

import (
	"context"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

func publishFailure(ctx context.Context, opmsClient opms.Client, task *types.Task, e error) {
	logger := log.FromContext(ctx)
	if task == nil || task.Data.Attributes == nil || task.Data.Attributes.JobId == "" {
		logger.Error("publish failure error: no job id was provided")
		return
	}
	inputError := util.DefaultPARError(e)
	err := opmsClient.PublishFailure(
		ctx,
		task.Data.Attributes.Client,
		task.Data.ID,
		task.Data.Attributes.JobId,
		task.GetFQN(),
		inputError.ErrorCode,
		inputError.Message,
		inputError.ExternalMessage,
	)
	if err != nil {
		logger.Error("publish failure error: unable to publish workflow task failure", log.ErrorField(err))
	}
}

func publishSuccess(ctx context.Context, opmsClient opms.Client, task *types.Task, output interface{}) {
	logger := log.FromContext(ctx)
	if task.Data.Attributes.JobId == "" {
		logger.Error("publish success error: no job id was provided")
		return
	}
	err := opmsClient.PublishSuccess(
		ctx,
		task.Data.Attributes.Client,
		task.Data.ID,
		task.Data.Attributes.JobId,
		task.GetFQN(),
		output,
		"",
	)
	if err != nil {
		logger.Error("publish success error: unable to publish workflow task success", log.ErrorField(err))
	}
}
