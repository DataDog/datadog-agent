// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

var errAuthoredScriptExecutionNotImplemented = errors.New("authored script execution is not implemented")

type AuthoredScripts struct {
	runAuthoredScript types.Action
}

func NewAuthoredScripts(enabled bool) *AuthoredScripts {
	return NewAuthoredScriptsWithCatalog(enabled, authoredscripts.NewStaticCatalog())
}

// NewAuthoredScriptsWithCatalog creates the bundle with an injected artifact
// catalog. NewAuthoredScripts retains the temporary static-catalog behavior.
func NewAuthoredScriptsWithCatalog(enabled bool, catalog authoredscripts.Catalog) *AuthoredScripts {
	return &AuthoredScripts{
		runAuthoredScript: NewRunAuthoredScriptHandlerWithCatalog(enabled, catalog),
	}
}

func (h *AuthoredScripts) GetAction(actionName string) types.Action {
	if actionName == "" {
		return nil
	}
	return h.runAuthoredScript
}
