// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package start

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pkgconfigutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// subscribeAgentConfig subscribes to the AGENT_CONFIG RC product to handle remote log level changes.
// It must be called before rcClient.Start() so the product is included in the first poll.
// The implementation mirrors the agentConfigUpdateCallback in comp/remote-config/rcclient/impl/rcclient.go.
func subscribeAgentConfig(
	rcClient *rcclient.Client,
	cfg config.Component,
) {
	rcClient.Subscribe(state.ProductAgentConfig, func(
		updates map[string]state.RawConfig,
		applyStateCallback func(string, state.ApplyStatus),
	) {
		mergedConfig, err := state.MergeRCAgentConfig(rcClient.UpdateApplyStatus, updates)
		if err != nil {
			return
		}

		var errList []error

		pkglog.Infof("A new log level configuration has been received through remote config: '%s'", mergedConfig.LogLevel)

		if cfg.GetSource("log_level") == pkgconfigmodel.SourceCLI {
			pkglog.Warnf("Remote config could not change the log level due to CLI override")
			return
		}

		if len(mergedConfig.LogLevel) == 0 {
			cfg.UnsetForSource("log_level", pkgconfigmodel.SourceRC)
			pkglog.Infof("Removing remote-config log level override, falling back to '%s'", cfg.Get("log_level"))
		} else {
			pkglog.Infof("Changing log level to '%s' through remote config", mergedConfig.LogLevel)
			if err := pkgconfigutils.SetLogLevel(mergedConfig.LogLevel, cfg, pkgconfigmodel.SourceRC); err != nil {
				errList = append(errList, err)
			}
		}

		errs := errors.Join(errList...)
		for cfgPath := range updates {
			if errs == nil {
				applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
			} else {
				applyStateCallback(cfgPath, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: fmt.Errorf("error while applying remote config: %s", errs.Error()).Error(),
				})
			}
		}
	})
}
