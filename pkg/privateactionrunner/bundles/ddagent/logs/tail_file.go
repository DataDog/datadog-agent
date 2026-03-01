// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// TailFileHandler implements tailFile: reads the last N lines of a host file.
type TailFileHandler struct{}

// NewTailFileHandler creates a new TailFileHandler.
func NewTailFileHandler() *TailFileHandler {
	return &TailFileHandler{}
}

type tailFileInputs struct {
	FilePath  string `json:"filePath"`
	LineCount int    `json:"lineCount,omitempty"`
}

// Run executes the tailFile action.
func (h *TailFileHandler) Run(
	_ context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[tailFileInputs](task)
	if err != nil {
		return nil, err
	}

	resolvedPath, err := sanitizeAndResolvePath(inputs.FilePath)
	if err != nil {
		return nil, err
	}

	lineCount := clampLineCount(inputs.LineCount)
	lines, count, err := tailFile(resolvedPath, lineCount)
	if err != nil {
		return nil, err
	}

	return &fileOutput{
		Lines:     lines,
		LineCount: count,
		FilePath:  inputs.FilePath,
	}, nil
}
