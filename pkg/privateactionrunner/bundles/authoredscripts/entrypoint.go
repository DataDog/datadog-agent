// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_authoredscripts

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	authoredscriptssupport "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type AuthoredScripts struct {
	runAuthoredScript types.Action
}

// NewAuthoredScripts creates the bundle and subscribes its catalog to Remote Config.
func NewAuthoredScripts(client rcclient.Client) (*AuthoredScripts, error) {
	catalog, err := authoredscriptssupport.NewRemoteCatalog(client, state.ProductUpdaterCatalogDD)
	if err != nil {
		return nil, err
	}
	return &AuthoredScripts{
		runAuthoredScript: NewRunAuthoredScriptHandler(catalog),
	}, nil
}

func (h *AuthoredScripts) GetAction(actionName string) types.Action {
	if actionName == "" {
		return nil
	}
	return h.runAuthoredScript
}
