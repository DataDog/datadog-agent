// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package snmptraps

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/log"
	trapsconfig "github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver"
	trapsserver "github.com/DataDog/datadog-agent/comp/snmptraps/server"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
	fxutil.TestBundle(t, Bundle,
		config.MockModule,
		hostnameimpl.MockModule,
		log.MockModule,
		fx.Provide(func() (*mocksender.MockSender, sender.Sender) {
			mockSender := mocksender.NewMockSender("mock-sender")
			mockSender.SetupAcceptAll()
			return mockSender, mockSender
		}),
	)
}
