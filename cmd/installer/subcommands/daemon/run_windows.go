// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/judwhite/go-svc"
	"go.uber.org/fx"
)

type windowsService struct {
	global     *command.GlobalParams
	shutdowner fx.Shutdowner
}

func runFxWrapper(global *command.GlobalParams) error {
	return svc.Run(&windowsService{
		global: global,
	})
}

func run(s *windowsService, shutdowner fx.Shutdowner, _ pid.Component, _ localapi.Component, _ telemetry.Component) error {
	s.shutdowner = shutdowner
	return nil
}

func (s *windowsService) Init(_ svc.Environment) error {
	return nil
}

func (s *windowsService) Start() error {
	return fxutil.OneShot(
		run,
		getCommonFxOption(s.global),
		fx.Supply(s),
	)
}

func (s *windowsService) Stop() error {
	_ = s.shutdowner.Shutdown()
	return nil
}
