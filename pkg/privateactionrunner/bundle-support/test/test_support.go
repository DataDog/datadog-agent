// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package test

import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

func NewTestTask(id, taskType string, attrs *types.Attributes) *types.Task {
	return &types.Task{
		Data: struct {
			ID         string            `json:"id,omitempty"`
			Type       string            `json:"type,omitempty"`
			Attributes *types.Attributes `json:"attributes,omitempty"`
		}{
			ID:         id,
			Type:       taskType,
			Attributes: attrs,
		},
	}
}
