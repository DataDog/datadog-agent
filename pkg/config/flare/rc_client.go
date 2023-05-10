// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package flare

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

type AgentTaskProvider struct {
	client *remote.Client

	configFilePath       string
	sysProbeConfFilePath string
	flareTaskProcessed   map[string]bool
}

const (
	agentTaskFlareType = "flare"
)

func NewAgentTaskProvider(name string, agentVersion string, configFilePath string, sysProbeConfFilePath string) (*AgentTaskProvider, error) {
	c, err := remote.NewUnverifiedGRPCClient(
		name, agentVersion, []data.Product{data.ProductAgentTask}, 1*time.Second,
	)
	if err != nil {
		return nil, err
	}

	path := configFilePath
	if len(path) != 0 || pkgconfig.Datadog == nil {
		path = config.DefaultConfPath
	}

	return &AgentTaskProvider{
		client:               c,
		configFilePath:       path,
		sysProbeConfFilePath: sysProbeConfFilePath,
		flareTaskProcessed:   map[string]bool{},
	}, nil
}

func (a *AgentTaskProvider) Start() {
	// TODO fix testing to put a non-nil client
	go func() {
		if a.client != nil {
			a.client.RegisterAgentTaskUpdate(a.agentTaskUpdateCallback)

			a.client.Start()
		}
	}()
}

func (a *AgentTaskProvider) makeFlare(flareComp flare.Component, config config.Component, sysprobeconfig sysprobeconfig.Component) error {
	// filePath, err := flareComp.Create(nil, nil)

	// pkglog.Warnf("Created a flare with error %s at path %s", err, filePath)

	// TOOD: use flareComp.Create instead of the post request
	pkglog.Warnf("[RCM] Ready to make the flare")

	// c := util.GetClient(false)
	// ipcAddress, err := pkgconfig.GetIPCAddress()
	// if err != nil {
	// 	return err
	// }

	// urlstr := fmt.Sprintf("https://%v:%v/agent/flare", ipcAddress, pkgconfig.Datadog.GetInt("cmd_port"))
	// pkglog.Warnf("Trying to reach %s", urlstr)

	// // Set session token
	// if err = util.SetAuthToken(); err != nil {
	// 	return err
	// }

	// r, err := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	// if err != nil {
	// 	return err
	// }

	// pkglog.Warnf("[RCM] flare request done: %s", r)

	return nil
}

func (a *AgentTaskProvider) agentTaskUpdateCallback(configs map[string]state.AgentTaskConfig) {
	for configPath, c := range configs {
		if c.Config.TaskType == agentTaskFlareType {
			// Check that the flare task wasn't already processed
			if !a.flareTaskProcessed[c.Config.UUID] {
				a.flareTaskProcessed[c.Config.UUID] = true
				pkglog.Debugf("Running agent flare task %s for %s", c.Config.UUID, configPath)
				a.client.UpdateApplyStatus(configPath, state.ApplyStatus{
					State: state.ApplyStateUnacknowledged,
				})

				var err error
				// Blocking?
				// fxutil.Run()
				// err = fxutil.OneShot(
				// 	a.makeFlare,
				// 	fx.Supply(core.BundleParams{
				// 		ConfigParams: config.NewParams(config.DefaultConfPath),
				// 		LogParams:    log.LogForOneShot("CORE", "off", false),
				// 		SysprobeConfigParams: sysprobeconfig.NewParams(
				// 			sysprobeconfig.WithSysProbeConfFilePath(a.sysProbeConfFilePath),
				// 		),
				// 	}),
				// 	fx.Supply(flare.NewLocalParams(
				// 		path.GetDistPath(),
				// 		path.PyChecksPath,
				// 		path.DefaultLogFile,
				// 		path.DefaultJmxLogFile,
				// 	)),
				// 	flare.Module,
				// 	core.Bundle,
				// )
				if err != nil {
					pkglog.Errorf("Couln't run the agent flare task: %s", err)
					a.client.UpdateApplyStatus(configPath, state.ApplyStatus{
						State: state.ApplyStateError,
						Error: err.Error(),
					})
				} else {
					pkglog.Debug("Flare task was executed")
					a.client.UpdateApplyStatus(configPath, state.ApplyStatus{
						State: state.ApplyStateAcknowledged,
					})
				}
			}
		}
	}
}
