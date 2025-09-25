// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_groups

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/pkg/errors"
)

type GitlabGroupsBundle struct{}

func NewGitlabGroups() types.Bundle {
	return &GitlabGroupsBundle{}
}

func (b *GitlabGroupsBundle) GetAction(actionName string) types.Action {
	return b
}

func (b *GitlabGroupsBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createGroup":
		return b.RunCreateGroup(ctx, task, credential)
	case "deleteGroup":
		return b.RunDeleteGroup(ctx, task, credential)
	case "getGroup":
		return b.RunGetGroup(ctx, task, credential)
	case "listGroups":
		return b.RunListGroups(ctx, task, credential)
	case "updateGroup":
		return b.RunUpdateGroup(ctx, task, credential)
	default:
		return nil, errors.Errorf("unknown action: %s", actionName)
	}
}
