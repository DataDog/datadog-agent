// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	authoredscripts "github.com/DataDog/datadog-agent/pkg/privateactionrunner/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// AuthoredScripts routes every action in the authored-scripts namespace to the package runner.
type AuthoredScripts struct {
	runAuthoredScript types.Action
}

func NewAuthoredScripts(runner *authoredscripts.Runner) *AuthoredScripts {
	return &AuthoredScripts{
		runAuthoredScript: NewRunAuthoredScriptHandler(runner),
	}
}

func (h *AuthoredScripts) GetAction(actionName string) types.Action {
	if actionName == "" {
		return nil
	}
	return h.runAuthoredScript
}
