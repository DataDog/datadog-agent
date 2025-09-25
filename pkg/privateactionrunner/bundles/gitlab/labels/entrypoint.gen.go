// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_labels

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabLabelsBundle struct{}

func NewGitlabLabels() types.Bundle {
	return &GitlabLabelsBundle{}
}

func (b *GitlabLabelsBundle) GetAction(actionName string) types.Action {
	return b
}

func (b *GitlabLabelsBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createLabel":
		return b.RunCreateLabel(ctx, task, credential)
	case "deleteLabel":
		return b.RunDeleteLabel(ctx, task, credential)
	case "getLabel":
		return b.RunGetLabel(ctx, task, credential)
	case "listLabels":
		return b.RunListLabels(ctx, task, credential)
	case "promoteLabel":
		return b.RunPromoteLabel(ctx, task, credential)
	case "subscribeToLabel":
		return b.RunSubscribeToLabel(ctx, task, credential)
	case "unsubscribeFromLabel":
		return b.RunUnsubscribeFromLabel(ctx, task, credential)
	case "updateLabel":
		return b.RunUpdateLabel(ctx, task, credential)
	default:
		return nil, fmt.Errorf("action not found %s", actionName)
	}
}
