// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package check

import (
	"github.com/DataDog/datadog-agent/comp/checks/windowseventlog"
	"github.com/DataDog/datadog-agent/comp/checks/windowseventlog/windowseventlogimpl"
	"github.com/DataDog/datadog-agent/comp/checks/winregistry"
	winregistryimpl "github.com/DataDog/datadog-agent/comp/checks/winregistry/impl"
	"go.uber.org/fx"
)

func getPlatformModules() fx.Option {
	return fx.Options(
		windowseventlogimpl.Module(),
		fx.Invoke(func(_ windowseventlog.Component) {}),
		winregistryimpl.Module(),
		fx.Invoke(func(_ winregistry.Component) {}),
	)
}
