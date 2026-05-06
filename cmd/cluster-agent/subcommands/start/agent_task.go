// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package start

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	clusterAgentFlare "github.com/DataDog/datadog-agent/pkg/flare/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const agentTaskTimeout = 5 * time.Minute

// subscribeAgentTask subscribes to the AGENT_TASK RC product to handle remote flare requests.
// It must be called before rcClient.Start() so the product is included in the first poll.
// The implementation mirrors the agentTaskUpdateCallback in comp/remote-config/rcclient/impl/rcclient.go.
func subscribeAgentTask(
	rcClient *rcclient.Client,
	cfg config.Component,
	statusComp status.Component,
	diagnoseComp diagnose.Component,
	ipcComp ipc.Component,
) {
	var (
		m             sync.Mutex
		taskProcessed = map[string]bool{}
	)

	rcClient.Subscribe(state.ProductAgentTask, func(
		updates map[string]state.RawConfig,
		applyStateCallback func(string, state.ApplyStatus),
	) {
		wg := &sync.WaitGroup{}
		wg.Add(len(updates))

		for originalConfigPath, originalConfig := range updates {
			go func(configPath string, c state.RawConfig) {
				pkglog.Debugf("[RemoteFlare] Agent task %s started", configPath)
				defer wg.Done()
				defer pkglog.Debugf("[RemoteFlare] Agent task %s completed", configPath)

				task, err := rcclienttypes.ParseConfigAgentTask(c.Config, c.Metadata)
				if err != nil {
					pkglog.Errorf("[RemoteFlare] Failed to parse AGENT_TASK config %s: %v", configPath, err)
					applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
					return
				}

				m.Lock()
				if taskProcessed[task.Config.UUID] {
					m.Unlock()
					return
				}
				taskProcessed[task.Config.UUID] = true
				m.Unlock()

				applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateUnacknowledged})

				var processed bool
				var taskErr error

				if rcclienttypes.TaskType(task.Config.TaskType) == rcclienttypes.TaskFlare {
					processed = true
					taskErr = clusterAgentFlare.HandleRCFlareTask(task, cfg, statusComp, diagnoseComp, ipcComp)
					if taskErr != nil {
						pkglog.Errorf("[RemoteFlare] Failed to create/send cluster-agent flare (UUID=%s): %v", task.Config.UUID, taskErr)
					}
				}

				if processed && taskErr != nil {
					applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: taskErr.Error()})
				} else if processed {
					pkglog.Infof("[RemoteFlare] Cluster-agent flare sent successfully (UUID=%s)", task.Config.UUID)
					applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
				} else {
					applyStateCallback(configPath, state.ApplyStatus{State: state.ApplyStateUnknown})
				}
			}(originalConfigPath, originalConfig)
		}

		// Mirror the reference implementation: wait for all goroutines with a top-level timeout.
		c := make(chan struct{})
		go func() {
			defer close(c)
			wg.Wait()
		}()
		select {
		case <-c:
			pkglog.Debugf("[RemoteFlare] All %d agent tasks applied successfully", len(updates))
		case <-time.After(agentTaskTimeout):
			pkglog.Warnf("[RemoteFlare] Timeout waiting for agent task(s) to complete")
		}
	})
}
