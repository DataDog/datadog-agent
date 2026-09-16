// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package com_datadoghq_authoredscripts

import (
	"context"
	"errors"

	authoredscriptssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// RunAuthoredScriptHandler is a stub on Windows, where authored-script execution is not supported.
type RunAuthoredScriptHandler struct{}

// NewRunAuthoredScriptHandler returns the Windows stub. Authored scripts are
// not supported on Windows, so the execution gate and catalog are unused.
func NewRunAuthoredScriptHandler(_ bool, _ authoredscriptssupport.Catalog) *RunAuthoredScriptHandler {
	return &RunAuthoredScriptHandler{}
}

func (h *RunAuthoredScriptHandler) Run(
	_ context.Context,
	_ *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return nil, errors.New("authored script execution is not implemented")
}
