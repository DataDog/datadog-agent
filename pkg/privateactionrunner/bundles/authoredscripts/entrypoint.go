// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	"errors"

	authoredscriptssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

var errAuthoredScriptExecutionDisabled = errors.New("authored-script execution is disabled")

// Dependencies contains the authored-script facilities supplied by PAR.
type Dependencies struct {
	Enabled bool
	Catalog authoredscriptssupport.Catalog
}

type AuthoredScripts struct {
	runAuthoredScript types.Action
}

// NewAuthoredScripts creates a disabled bundle for callers that do not provide
// the opt-in authored-script facilities.
func NewAuthoredScripts() *AuthoredScripts {
	return NewAuthoredScriptsWithDependencies(Dependencies{})
}

// NewAuthoredScriptsWithDependencies creates a bundle with its runtime facilities.
func NewAuthoredScriptsWithDependencies(dependencies Dependencies) *AuthoredScripts {
	return &AuthoredScripts{
		runAuthoredScript: NewRunAuthoredScriptHandler(dependencies),
	}
}

func (h *AuthoredScripts) GetAction(actionName string) types.Action {
	if actionName == "" {
		return nil
	}
	return h.runAuthoredScript
}
