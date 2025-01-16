// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package config

import (
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	corecompcfg "github.com/DataDog/datadog-agent/comp/core/config"
	rc "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func remote(c corecompcfg.Component, ipcAddress string, at authtoken.Component) (config.RemoteClient, error) {
	return rc.NewGRPCClient(
		ipcAddress,
		pkgconfigsetup.GetIPCPort(),
		at.Get,
		at.GetTLSClientConfig,
		rc.WithAgent(rcClientName, version.AgentVersion),
		rc.WithProducts(state.ProductAPMSampling, state.ProductAgentConfig),
		rc.WithPollInterval(rcClientPollInterval),
		rc.WithDirectorRootOverride(c.GetString("site"), c.GetString("remote_configuration.director_root")),
	)

}
