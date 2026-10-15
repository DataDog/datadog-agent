// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package fx provides the fx module for the serverlessinit component
package fx

import (
	"github.com/google/uuid"
	"go.uber.org/fx"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx-none"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	iadef "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/def"
	serverlessinitimpl "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module defines the fx options for this component. It also carries the
// inventoryagent adapters that only the serverless-init graph needs — the
// serializer through the demultiplexer, the noop IPC client, and the absent
// system-probe configuration — plus the serverless capabilities: one uuid for
// the process lifetime, since serverless containers do not share a host GUID.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(serverlessinitimpl.NewComponent),
		fx.Provide(func(d aggregator.Demultiplexer) serializer.MetricSerializer { return d.Serializer() }),
		ipcfx.Module(),
		fx.Provide(func(c ipc.Component) ipc.HTTPClient { return c.GetClient() }),
		fx.Provide(func() option.Option[sysprobeconfig.Component] { return option.None[sysprobeconfig.Component]() }),
		fx.Provide(func() *iadef.Capabilities {
			id := uuid.New().String()
			return iadef.NewServerlessCapabilities(func() string { return id })
		}),
	)
}
