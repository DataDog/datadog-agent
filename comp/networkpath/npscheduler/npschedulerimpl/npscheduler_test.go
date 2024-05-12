// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npschedulerimpl

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler"
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler/npschedulerimpl/common"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// testOptions is a fx collection of common dependencies for all tests
var testOptions = fx.Options(
	Module(),
	forwarderimpl.MockModule(),
	demultiplexerimpl.MockModule(),
	defaultforwarder.MockModule(),
	core.MockBundle(),
	eventplatformimpl.MockModule(),
)

func newTestNpScheduler(t *testing.T, sysConfigs map[string]any) (*fxtest.App, *npSchedulerImpl) {
	var component npscheduler.Component
	app := fxtest.New(t, fx.Options(
		testOptions,
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		fx.Replace(sysprobeconfigimpl.MockParams{Overrides: sysConfigs}),
		fx.Populate(&component),
	))
	npScheduler := component.(*npSchedulerImpl)

	require.NotNil(t, npScheduler)
	require.NotNil(t, app)
	return app, npScheduler
}

func TestStartServerAndStopNpScheduler(t *testing.T) {
	sysConfigs := map[string]any{
		"network_path.enabled": true,
	}
	app, npScheduler := newTestNpScheduler(t, sysConfigs)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	utillog.SetupLogger(l, "debug")

	assert.False(t, npScheduler.running)
	app.RequireStart()
	assert.True(t, npScheduler.running)
	app.RequireStop()
	assert.False(t, npScheduler.running)

	w.Flush()
	logs := b.String()

	assert.Equal(t, 1, strings.Count(logs, "Start NpScheduler"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Starting listening for pathtests"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Starting flush loop"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Starting workers"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Starting worker #0"), logs)

	assert.Equal(t, 1, strings.Count(logs, "Stopped listening for pathtests"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Stopped flush loop"), logs)
	assert.Equal(t, 1, strings.Count(logs, "Stop NpScheduler"), logs)
}

func Test_newNpSchedulerImpl_defaultConfigs(t *testing.T) {
	sysConfigs := map[string]any{
		"network_path.enabled": true,
	}

	_, npScheduler := newTestNpScheduler(t, sysConfigs)

	assert.Equal(t, true, npScheduler.enabled)
	assert.Equal(t, 4, npScheduler.workers)
	assert.Equal(t, 1000, cap(npScheduler.pathtestInputChan))
	assert.Equal(t, 1000, cap(npScheduler.pathtestProcessChan))
}

func Test_newNpSchedulerImpl_overrideConfigs(t *testing.T) {
	sysConfigs := map[string]any{
		"network_path.enabled":           true,
		"network_path.workers":           2,
		"network_path.input_chan_size":   300,
		"network_path.process_chan_size": 400,
	}

	_, npScheduler := newTestNpScheduler(t, sysConfigs)

	assert.Equal(t, true, npScheduler.enabled)
	assert.Equal(t, 2, npScheduler.workers)
	assert.Equal(t, 300, cap(npScheduler.pathtestInputChan))
	assert.Equal(t, 400, cap(npScheduler.pathtestProcessChan))
}

func Test_npSchedulerImpl_ScheduleConns(t *testing.T) {
	_, npScheduler := newTestNpScheduler(t, map[string]any{"network_path.enabled": true})
	conns := []*model.Connection{
		{
			// traffic other than outgoing is ignore
			Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
			Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
			Direction: model.ConnectionDirection_incoming,
		},
		{
			Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
			Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
		},
	}
	npScheduler.ScheduleConns(conns)
	pathtest := <-npScheduler.pathtestInputChan
	assert.Equal(t, &common.Pathtest{Hostname: "127.0.0.4", Port: uint16(80)}, pathtest)

	// TODO: CONTINUE TESTING MORE THINGS HERE
	// TODO: CONTINUE TESTING MORE THINGS HERE
	// TODO: CONTINUE TESTING MORE THINGS HERE
	// TODO: CONTINUE TESTING MORE THINGS HERE
}
