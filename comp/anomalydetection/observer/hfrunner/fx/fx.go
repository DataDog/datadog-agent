// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx defines the fx options for the hfrunner component.
package fx

import (
	hfrunnerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/hfrunner/def"
	hfrunnerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/hfrunner/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
// We construct fxutil.Module directly because fxutil.Component uses runtime call-stack
// inspection that only supports paths up to 5 levels deep from comp/, and this
// component lives at comp/anomalydetection/observer/hfrunner (6 levels deep).
func Module() fxutil.Module {
	opts := []fx.Option{
		fxutil.ProvideComponentConstructor(hfrunnerimpl.NewComponent),
		fx.Provide(func(c hfrunnerdef.Component) option.Option[hfrunnerdef.Component] {
			return option.New(c)
		}),
	}
	return fxutil.Module{
		Option:  fx.Module("comp/anomalydetection/observer/hfrunner", opts...),
		Options: opts,
	}
}
