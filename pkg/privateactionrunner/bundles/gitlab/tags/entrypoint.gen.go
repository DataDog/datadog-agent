// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_tags

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabTagsBundle struct{}

func NewGitlabTags() types.Bundle {
	return &GitlabTagsBundle{}
}

func (b *GitlabTagsBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "createTag":
		return b.RunCreateTag(ctx, task, credential)
	case "deleteTag":
		return b.RunDeleteTag(ctx, task, credential)
	case "getTag":
		return b.RunGetTag(ctx, task, credential)
	case "listTags":
		return b.RunListTags(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (h *GitlabTagsBundle) GetAction(actionName string) types.Action {
	return h
}
