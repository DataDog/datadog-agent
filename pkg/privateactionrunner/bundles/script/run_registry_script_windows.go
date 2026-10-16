// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package com_datadoghq_script

import (
	"context"
	"errors"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// RunRegistryScriptHandler is unsupported on Windows, like RC-authored scripts.
type RunRegistryScriptHandler struct{}

func NewRunRegistryScriptHandler(_ *http.Client, _ []string) *RunRegistryScriptHandler {
	return &RunRegistryScriptHandler{}
}

func (*RunRegistryScriptHandler) Run(context.Context, *types.Task, *privateconnection.PrivateCredentials) (interface{}, error) {
	return nil, errors.New("registry script execution is not supported on Windows")
}
