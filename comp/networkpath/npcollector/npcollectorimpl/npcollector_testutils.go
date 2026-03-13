// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"fmt"
	"math/rand"
	"net"
	"net/netip"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	rdnsqueriermock "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
)

// MockTimeNow mocks time.Now
var MockTimeNow = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

// testOptions is a fx collection of common dependencies for all tests
var testOptions = fx.Options(
	Module(),
	forwarderimpl.MockModule(),
	demultiplexerimpl.MockModule(),
	defaultforwarder.MockModule(),
	eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
	eventplatformreceiverimpl.Module(),
	rdnsqueriermock.MockModule(),
	logscompression.MockModule(),
	telemetryimpl.MockModule(),
	hostnameimpl.MockModule(),
	fx.Provide(delegatedauthmock.New),
)

func newTestNpCollector(t testing.TB, agentConfigs map[string]any, statsdClient statsd.ClientInterface, tr traceroute.Component) (*fxtest.App, *npCollectorImpl) {
	var component npcollector.Component
	app := fxtest.New(t, fx.Options(
		testOptions,
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		fx.Provide(func() config.Component { return config.NewMockWithOverrides(t, agentConfigs) }),
		fx.Populate(&component),
		fx.Provide(func() statsd.ClientInterface {
			return statsdClient
		}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() traceroute.Component { return tr }),
	))
	npCollector := component.(*npCollectorImpl)

	require.NotNil(t, npCollector)
	require.NotNil(t, app)
	return app, npCollector
}

func createConns(numberOfConns int) []npmodel.NetworkPathConnection {
	var conns []npmodel.NetworkPathConnection
	for i := 0; i < numberOfConns; i++ {
		conns = append(conns, npmodel.NetworkPathConnection{
			Source:    netip.MustParseAddrPort(fmt.Sprintf("10.0.0.%d:30000", i)),
			Dest:      netip.MustParseAddrPort(fmt.Sprintf("10.0.1.%d:80", i)),
			Direction: model.ConnectionDirection_outgoing,
		})
	}

	return conns
}

func createBenchmarkConns(numberOfConns int, tcpPercent int) []npmodel.NetworkPathConnection {
	port := rand.Intn(65535-1) + 1
	connType := model.ConnectionType_udp
	if rand.Intn(100) < tcpPercent {
		connType = model.ConnectionType_tcp
	}
	var conns []npmodel.NetworkPathConnection
	for i := 0; i < numberOfConns; i++ {
		conns = append(conns, npmodel.NetworkPathConnection{
			Source:    netip.MustParseAddrPort(fmt.Sprintf("127.0.0.%d:30000", i)),
			Dest:      netip.MustParseAddrPort(fmt.Sprintf("%s:%d", randomPublicIP(), int32(port))),
			Direction: model.ConnectionDirection_outgoing,
			Type:      connType,
		})
	}
	return conns
}

func randomPublicIP() string {
	var ip string
	for {
		ip = fmt.Sprintf("%d.%d.%d.%d", rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256))
		parsedIP := net.ParseIP(ip)
		if parsedIP != nil && !parsedIP.IsLoopback() && !parsedIP.IsPrivate() {
			break
		}
	}
	return ip
}

func waitForProcessedPathtests(npCollector *npCollectorImpl, timeout time.Duration, processedCount uint64) {
	timeoutChan := time.After(timeout)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-timeoutChan:
			return
		case <-tick.C:
			if npCollector.processedTracerouteCount.Load() >= processedCount {
				return
			}
		}
	}
}
