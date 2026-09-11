// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !kubeapiserver && !windows

package privateactionrunnerimpl

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
)

func newAuthoredScriptsCatalog(configuration *config.Config, client rcclient.Client) (authoredscripts.Catalog, error) {
	if !configuration.AuthoredScriptsEnabled {
		return nil, nil
	}
	catalog := authoredscripts.NewRemoteConfigCatalog(client)
	if err := catalog.Start(); err != nil {
		return nil, err
	}
	return catalog, nil
}
