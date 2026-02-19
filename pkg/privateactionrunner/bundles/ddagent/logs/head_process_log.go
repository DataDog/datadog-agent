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

// HeadProcessLogHandler implements headProcessLog: reads the first N lines of a host file.
type HeadProcessLogHandler struct{}

// NewHeadProcessLogHandler creates a new HeadProcessLogHandler.
func NewHeadProcessLogHandler() *HeadProcessLogHandler {
	return &HeadProcessLogHandler{}
}

type headProcessLogInputs struct {
	FilePath  string `json:"filePath"`
	LineCount int    `json:"lineCount,omitempty"`
}

type processLogOutput struct {
	Lines     string `json:"lines"`
	LineCount int    `json:"lineCount"`
	FilePath  string `json:"filePath"`
}

// Run executes the headProcessLog action.
func (h *HeadProcessLogHandler) Run(
	_ context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[headProcessLogInputs](task)
	if err != nil {
		return nil, err
	}

	resolvedPath, err := sanitizeAndResolvePath(inputs.FilePath)
	if err != nil {
		return nil, err
	}

	lineCount := clampLineCount(inputs.LineCount)
	lines, count, err := headFile(resolvedPath, lineCount)
	if err != nil {
		return nil, err
	}

	return &processLogOutput{
		Lines:     lines,
		LineCount: count,
		FilePath:  inputs.FilePath,
	}, nil
}
