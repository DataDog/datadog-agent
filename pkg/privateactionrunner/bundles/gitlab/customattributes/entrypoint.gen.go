// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_gitlab_custom_attributes

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type GitlabCustomAttributesBundle struct{}

func NewGitlabCustomAttributes() types.Bundle {
	return &GitlabCustomAttributesBundle{}
}

func (b *GitlabCustomAttributesBundle) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "deleteCustomGroupAttribute":
		return b.RunDeleteCustomGroupAttribute(ctx, task, credential)
	case "deleteCustomProjectAttribute":
		return b.RunDeleteCustomProjectAttribute(ctx, task, credential)
	case "deleteCustomUserAttribute":
		return b.RunDeleteCustomUserAttribute(ctx, task, credential)
	case "getCustomGroupAttribute":
		return b.RunGetCustomGroupAttribute(ctx, task, credential)
	case "getCustomProjectAttribute":
		return b.RunGetCustomProjectAttribute(ctx, task, credential)
	case "getCustomUserAttribute":
		return b.RunGetCustomUserAttribute(ctx, task, credential)
	case "listCustomGroupAttributes":
		return b.RunListCustomGroupAttributes(ctx, task, credential)
	case "listCustomProjectAttributes":
		return b.RunListCustomProjectAttributes(ctx, task, credential)
	case "listCustomUserAttributes":
		return b.RunListCustomUserAttributes(ctx, task, credential)
	case "setCustomGroupAttribute":
		return b.RunSetCustomGroupAttribute(ctx, task, credential)
	case "setCustomProjectAttribute":
		return b.RunSetCustomProjectAttribute(ctx, task, credential)
	case "setCustomUserAttribute":
		return b.RunSetCustomUserAttribute(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (b *GitlabCustomAttributesBundle) GetAction(actionName string) types.Action {
	return b
}
