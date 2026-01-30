// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package run

import (
	"go.uber.org/fx"

	connectionsforwarderfx "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
	networktracer "github.com/DataDog/datadog-agent/comp/system-probe/networktracer/fx"
)

func getPlatformModules() fx.Option {
	return fx.Options(
		// TODO should this be a bundle?
		connectionsforwarderfx.Module(),
		eventplatformreceiverimpl.Module(),
		eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
		rdnsquerierfx.Module(),
		npcollectorimpl.Module(),
		// above are deps for system-probe module below
		networktracer.Module(),
	)
}
