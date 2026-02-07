// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package run

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	localtraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-local"
	traceroute "github.com/DataDog/datadog-agent/comp/system-probe/traceroute/fx"
)

func getPlatformModules() fx.Option {
	return fx.Options(
		remotehostnameimpl.Module(),
		localtraceroute.Module(),
		traceroute.Module(),
	)
}
