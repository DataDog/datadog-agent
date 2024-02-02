// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package config

import (
	"fmt"
	"time"

	corecompcfg "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	rc "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"

	//nolint:revive // TODO(APM) Fix revive linter

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// team: agent-apm
const (
	// rcClientPollInterval is the default poll interval for remote configuration clients. 1 second ensures that
	// clients remain up to date without paying too much of a performance cost (polls that contain no updates are cheap)
	rcClientPollInterval = time.Second * 1
)

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

	ipcAddress, err := coreconfig.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	orch := fargate.GetOrchestrator() // Needs to be after loading config, because it relies on feature auto-detection
	cfg.FargateOrchestrator = config.FargateOrchestratorName(orch)
	if p := coreconfig.Datadog.GetProxies(); p != nil {
		cfg.Proxy = httputils.GetProxyTransportFunc(p, c)
	}
	if coreconfig.IsRemoteConfigEnabled(coreConfigObject) && coreConfigObject.GetBool("remote_configuration.apm_sampling.enabled") {
		client, err := rc.NewGRPCClient(
			ipcAddress,
			coreconfig.GetIPCPort(),
			security.FetchAuthToken,
			rc.WithAgent(rcClientName, version.AgentVersion),
			rc.WithProducts([]data.Product{data.ProductAPMSampling, data.ProductAgentConfig}),
			rc.WithPollInterval(rcClientPollInterval),
			rc.WithDirectorRootOverride(c.GetString("remote_configuration.director_root")),
		)
		if err != nil {
			log.Errorf("Error when subscribing to remote config management %v", err)
		} else {
			cfg.RemoteConfigClient = client
		}
	}
	cfg.ContainerTags = containerTagsFunc
	cfg.ContainerProcRoot = coreConfigObject.GetString("container_proc_root")
	return cfg, nil
}
