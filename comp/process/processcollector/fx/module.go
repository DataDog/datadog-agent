// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx creates the process collector module
package fx

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	processcollectorimpl "github.com/DataDog/datadog-agent/comp/process/processcollector/processcollectorimpl"
)

// Module specifies the processcollector module.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(processcollectorimpl.NewCollector))
}
