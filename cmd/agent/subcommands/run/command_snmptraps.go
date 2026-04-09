// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/snmptraps"
	snmptrapsServer "github.com/DataDog/datadog-agent/comp/snmptraps/server"
)

func getSnmptrapsOptions() fx.Option {
	return fx.Options(
		snmptraps.Bundle(),
		fx.Invoke(func(_ snmptrapsServer.Component) {}),
	)
}
