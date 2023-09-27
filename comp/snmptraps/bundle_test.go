// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package snmptraps

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/ndmtmp"
	trapsconfig "github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver"
	trapsserver "github.com/DataDog/datadog-agent/comp/snmptraps/server"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

type deps struct {
	fx.In
	Config    trapsconfig.Component
	Formatter formatter.Component
	Forwarder forwarder.Component
	Listener  listener.Component
	Resolver  oidresolver.Component
	Server    trapsserver.Component
	Status    status.Component
}

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		config.MockModule,
		hostname.MockModule,
		log.MockModule,
		ndmtmp.MockBundle,
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		// instantiate all of the ndmtmp components, since this is not done
		// automatically.
		fx.Invoke(func(deps) {}),
		Bundle,
	))
}
