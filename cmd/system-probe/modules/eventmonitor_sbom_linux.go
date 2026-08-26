// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package modules

import (
	"context"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
)

// wireSBOMUsage gives the SBOM resolver its source of file indexes and returns
// the function that opens the streams to the core agent, to be called once the
// event monitor is built.
func wireSBOMUsage(opts *eventmonitor.Opts, cfg *secconfig.Config, ipcComp ipc.Component) func(context.Context) {
	if !cfg.RuntimeSecurity.SBOMResolverEnabled {
		return func(context.Context) {}
	}

	client := secmodule.NewSBOMUsageClient(
		ipcComp,
		pkgconfigsetup.Datadog().GetString("cmd_host"),
		pkgconfigsetup.Datadog().GetInt("cmd_port"),
	)
	opts.ProbeOpts.SBOMIndexSource = client

	return client.Start
}
