// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package fx creates the process collector module
package fx

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	processcollector "github.com/DataDog/datadog-agent/comp/process/processcollector/def"
)

// Module specifies the fallback processcollector module.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() optional.Option[processcollector.Component] {
			return optional.NewNoneOption[processcollector.Component]()
		}))
}
