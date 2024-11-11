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
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	rdnsqueriermock "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
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
	core.MockBundle(),
	eventplatformimpl.MockModule(),
	rdnsqueriermock.MockModule(),
)

func newTestNpCollector(t fxtest.TB, agentConfigs map[string]any) (*fxtest.App, *npCollectorImpl) {
	var component npcollector.Component
	app := fxtest.New(t, fx.Options(
		testOptions,
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		fx.Replace(config.MockParams{Overrides: agentConfigs}),
		fx.Populate(&component),
	))
	npCollector := component.(*npCollectorImpl)

	require.NotNil(t, npCollector)
	require.NotNil(t, app)
	return app, npCollector
}

func createConns(numberOfConns int) []*model.Connection {
	var conns []*model.Connection
	for i := 0; i < numberOfConns; i++ {
		conns = append(conns, &model.Connection{
			Laddr:     &model.Addr{Ip: fmt.Sprintf("10.0.0.%d", i), Port: int32(30000)},
			Raddr:     &model.Addr{Ip: fmt.Sprintf("10.0.1.%d", i), Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
		})
	}
	return conns
}

func createBenchmarkConns(numberOfConns int, tcpPercent int) []*model.Connection {
	port := rand.Intn(65535-1) + 1
	connType := model.ConnectionType_udp
	if rand.Intn(100) < tcpPercent {
		connType = model.ConnectionType_tcp
	}
	var conns []*model.Connection
	for i := 0; i < numberOfConns; i++ {
		conns = append(conns, &model.Connection{
			Laddr:     &model.Addr{Ip: fmt.Sprintf("127.0.0.%d", i), Port: int32(30000)},
			Raddr:     &model.Addr{Ip: randomPublicIP(), Port: int32(port)},
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
