// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	authoredscriptssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type AuthoredScripts struct {
	runAuthoredScript types.Action
}

// NewAuthoredScripts creates the bundle with its artifact catalog supplied by the bundle registry.
func NewAuthoredScripts(catalog authoredscriptssupport.Catalog) *AuthoredScripts {
	return &AuthoredScripts{
		runAuthoredScript: NewRunAuthoredScriptHandler(catalog),
	}
}

func (h *AuthoredScripts) GetAction(actionName string) types.Action {
	if actionName == "" {
		return nil
	}
	return h.runAuthoredScript
}
