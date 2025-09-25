// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_members

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabMembersBundle struct {
}

func NewGitlabMembers() types.Bundle {
	return &GitlabMembersBundle{}
}

func (b *GitlabMembersBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "listProjectMembers":
		return b.RunListProjectMembers(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (b *GitlabMembersBundle) GetAction(actionName string) types.Action {
	return b
}
