// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package check

import (
	windowseventlog "github.com/DataDog/datadog-agent/comp/checks/windowseventlog/def"
	windowseventlogfx "github.com/DataDog/datadog-agent/comp/checks/windowseventlog/fx"
	"github.com/DataDog/datadog-agent/comp/checks/winregistry"
	winregistryimpl "github.com/DataDog/datadog-agent/comp/checks/winregistry/impl"
	publishermetadatacachefx "github.com/DataDog/datadog-agent/comp/publishermetadatacache/fx"
	"go.uber.org/fx"
)

func getPlatformModules() fx.Option {
	return fx.Options(
		windowseventlogfx.Module(),
		fx.Invoke(func(_ windowseventlog.Component) {}),
		winregistryimpl.Module(),
		fx.Invoke(func(_ winregistry.Component) {}),
		publishermetadatacachefx.Module(),
	)
}
