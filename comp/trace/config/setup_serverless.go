// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"

	corecompcfg "github.com/DataDog/datadog-agent/comp/core/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"

	//nolint:revive // TODO(APM) Fix revive linter

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// go:build serverless

// team: agent-apm

func prepareConfig(c corecompcfg.Component) (*config.AgentConfig, error) {
	cfg := config.New()
	cfg.DDAgentBin = defaultDDAgentBin
	cfg.AgentVersion = version.AgentVersion
	cfg.GitCommit = version.Commit

	// the core config can be assumed to already be set-up as it has been
	// injected as a component dependency
	// TODO: do not interface directly with pkg/config anywhere
	coreConfigObject := c.Object()
	if coreConfigObject == nil {
		//nolint:revive // TODO(APM) Fix revive linter
		return nil, fmt.Errorf("no core config found! Bailing out.")
	}

	if !coreConfigObject.GetBool("disable_file_logging") {
		cfg.LogFilePath = DefaultLogFilePath
	}

	orch := fargate.GetOrchestrator() // Needs to be after loading config, because it relies on feature auto-detection
	cfg.FargateOrchestrator = config.FargateOrchestratorName(orch)
	if p := coreconfig.Datadog.GetProxies(); p != nil {
		cfg.Proxy = httputils.GetProxyTransportFunc(p, c)
	}
	cfg.ContainerTags = containerTagsFunc
	cfg.ContainerProcRoot = coreConfigObject.GetString("container_proc_root")
	return cfg, nil
}
