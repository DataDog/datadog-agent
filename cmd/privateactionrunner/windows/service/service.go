// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package service

import (
	"context"
	"errors"
	"path/filepath"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	privateactionrunnerfx "github.com/DataDog/datadog-agent/comp/privateactionrunner/fx"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/rcclientimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice/rcserviceimpl"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	ServiceName = "datadog-private-action-runner"
)

var (
	defaultConfPath = filepath.Join(defaultpaths.ConfPath, "datadog.yaml")
)

type windowsService struct {
	servicemain.DefaultSettings
}

func NewService() servicemain.Service {
	return &windowsService{}
}

func (s *windowsService) Name() string {
	return ServiceName
}

func (s *windowsService) Init() error {
	return nil
}

func (s *windowsService) Run(ctx context.Context) error {
	err := fxutil.Run(
		fx.Provide(func() context.Context { return ctx }),
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(defaultConfPath),
			LogParams:    log.ForDaemon("PRIV-ACTION", "private_action_runner.log_file", ""),
		}),
		core.Bundle(),
		secretsnoopfx.Module(),
		fx.Provide(func(c config.Component) settings.Params {
			return settings.Params{
				Settings: map[string]settings.RuntimeSetting{
					"log_level": commonsettings.NewLogLevelRuntimeSetting(),
				},
				Config: c,
			}
		}),
		settingsimpl.Module(),
		ipcfx.ModuleReadWrite(),
		rcserviceimpl.Module(),
		rcclientimpl.Module(),
		fx.Supply(rcclient.Params{AgentName: "private-action-runner", AgentVersion: version.AgentVersion}),
		privateactionrunnerfx.Module(),
	)

	if errors.Is(err, privateactionrunner.ErrNotEnabled) {
		// If private action runner is not enabled, exit cleanly
		return servicemain.ErrCleanStopAfterInit
	}

	return err
}

