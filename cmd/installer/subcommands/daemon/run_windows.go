// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"github.com/judwhite/go-svc"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
)

type windowsService struct {
	shutdowner fx.Shutdowner
}

func run(shutdowner fx.Shutdowner, _ pid.Component, _ localapi.Component, _ telemetry.Component) error {
	if err := svc.Run(&windowsService{
		shutdowner: shutdowner,
	}); err != nil {
		_ = shutdowner.Shutdown()
	}
	return nil
}

func (s *windowsService) Init(_ svc.Environment) error {
	return nil
}

func (s *windowsService) Start() error {
	return nil
}

func (s *windowsService) Stop() error {
	_ = s.shutdowner.Shutdown()
	return nil
}
