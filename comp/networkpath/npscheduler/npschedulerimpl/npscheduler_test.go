// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npschedulerimpl

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler"
	"github.com/DataDog/datadog-agent/comp/networkpath/npscheduler/npschedulerimpl/common"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
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

func createConns(numberOfConns int) []*model.Connection {
	var conns []*model.Connection
	for i := 0; i < numberOfConns; i++ {
		conns = append(conns, &model.Connection{
			Laddr:     &model.Addr{Ip: fmt.Sprintf("127.0.0.%d", i), Port: int32(30000)},
			Raddr:     &model.Addr{Ip: fmt.Sprintf("127.0.1.%d", i), Port: int32(80)},
			Direction: model.ConnectionDirection_outgoing,
		})
	}
	return conns
}

func Test_NpScheduler_StartAndStop(t *testing.T) {
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

}

func Test_npSchedulerImpl_ScheduleConns1(t *testing.T) {
	type logCount struct {
		log   string
		count int
	}
	defaultSysConfigs := map[string]any{
		"network_path.enabled": true,
	}
	tests := []struct {
		name              string
		conns             []*model.Connection
		noInputChan       bool
		sysConfigs        map[string]any
		expectedPathtests []*common.Pathtest
		expectedLogs      []logCount
	}{
		{
			name:              "zero conn",
			sysConfigs:        defaultSysConfigs,
			conns:             []*model.Connection{},
			expectedPathtests: []*common.Pathtest{},
		},
		{
			name:       "one outgoing conn",
			sysConfigs: defaultSysConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "127.0.0.4", Port: uint16(80)},
			},
		},
		{
			name:       "only non-outgoing conns",
			sysConfigs: defaultSysConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
					Direction: model.ConnectionDirection_incoming,
				},
				{
					Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_incoming,
				},
			},
			expectedPathtests: []*common.Pathtest{},
		},
		{
			name:       "ignore non-outgoing conn",
			sysConfigs: defaultSysConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.2", Port: int32(80)},
					Direction: model.ConnectionDirection_incoming,
				},
				{
					Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "127.0.0.4", Port: uint16(80)},
			},
		},
		{
			name:        "no input chan",
			sysConfigs:  defaultSysConfigs,
			noInputChan: true,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
				},
			},
			expectedPathtests: []*common.Pathtest{},
			expectedLogs: []logCount{
				{"[ERROR] ScheduleConns: Error scheduling pathtests: no input channel, please check that network path is enabled", 1},
			},
		},
		{
			name: "input chan is full",
			sysConfigs: map[string]any{
				"network_path.enabled":         true,
				"network_path.input_chan_size": 1,
			},
			conns:             createConns(10),
			expectedPathtests: []*common.Pathtest{},
			expectedLogs: []logCount{
				{"Error scheduling pathtests: scheduler input channel is full", 9},
			},
		},
		{
			name:       "only ipv4 supported",
			sysConfigs: defaultSysConfigs,
			conns: []*model.Connection{
				{
					Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "::1", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
				},
				{
					Laddr:     &model.Addr{Ip: "127.0.0.1", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "::1", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
				},
				{
					Laddr:     &model.Addr{Ip: "127.0.0.3", Port: int32(30000)},
					Raddr:     &model.Addr{Ip: "127.0.0.4", Port: int32(80)},
					Direction: model.ConnectionDirection_outgoing,
				},
			},
			expectedPathtests: []*common.Pathtest{
				{Hostname: "127.0.0.4", Port: uint16(80)},
			},
			expectedLogs: []logCount{
				{"Only IPv4 is currently supported. Address not supported: ::1", 2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, npScheduler := newTestNpScheduler(t, tt.sysConfigs)
			if tt.noInputChan {
				npScheduler.pathtestInputChan = nil
			}

			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
			assert.Nil(t, err)
			utillog.SetupLogger(l, "debug")

			stats := &teststatsd.Client{}
			npScheduler.statsdClient = stats

			npScheduler.ScheduleConns(tt.conns)

			actualPathtests := []*common.Pathtest{}
			for i := 0; i < len(tt.expectedPathtests); i++ {
				select {
				case pathtest := <-npScheduler.pathtestInputChan:
					actualPathtests = append(actualPathtests, pathtest)
				case <-time.After(200 * time.Millisecond):
					assert.Fail(t, fmt.Sprintf("Not enough pathtests: expected=%d but actual=%d", len(tt.expectedPathtests), len(actualPathtests)))
				}
			}

			assert.Equal(t, tt.expectedPathtests, actualPathtests)

			// Flush logs
			w.Flush()
			logs := b.String()

			// Test metrics
			var scheduleDurationMetric teststatsd.MetricsArgs
			calls := stats.GaugeCalls
			for _, call := range calls {
				if call.Name == "datadog.network_path.scheduler.schedule_duration" {
					scheduleDurationMetric = call
				}
			}
			assert.Less(t, scheduleDurationMetric.Value, float64(5)) // we can't easily assert precise value, hence we are only asserting that it's a low value e.g. 5 seconds
			scheduleDurationMetric.Value = 0                         // We need to reset the metric value to ease testing time duration
			assert.Equal(t, teststatsd.MetricsArgs{Name: "datadog.network_path.scheduler.schedule_duration", Value: 0, Tags: nil, Rate: 1}, scheduleDurationMetric)

			// Test using logs
			for _, expectedLog := range tt.expectedLogs {
				assert.Equal(t, expectedLog.count, strings.Count(logs, expectedLog.log), logs)
			}
		})
	}
}
