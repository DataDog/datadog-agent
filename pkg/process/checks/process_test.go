// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/process/metadata"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	metricsmock "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/subscriptions"
)

func processCheckWithMockProbe(t *testing.T) (*ProcessCheck, *mocks.Probe) {
	t.Helper()
	probe := mocks.NewProbe(t)
	sysInfo := &model.SystemInfo{
		Cpus: []*model.CPUInfo{
			{CoreId: "1"},
			{CoreId: "2"},
			{CoreId: "3"},
			{CoreId: "4"},
		},
	}
	hostInfo := &HostInfo{
		SystemInfo: sysInfo,
	}
	serviceExtractorEnabled := true
	useWindowsServiceName := true
	useImprovedAlgorithm := false
	serviceExtractor := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)

	return &ProcessCheck{
		probe:             probe,
		scrubber:          procutil.NewDefaultDataScrubber(),
		hostInfo:          hostInfo,
		containerProvider: mockContainerProvider(t),
		sysProbeConfig:    &SysProbeConfig{},
		checkCount:        0,
		skipAmount:        2,
		serviceExtractor:  serviceExtractor,
		extractors:        []metadata.Extractor{serviceExtractor},
	}, probe
}

// TODO: create a centralized, easy way to mock this
func mockContainerProvider(t *testing.T) proccontainers.ContainerProvider {
	t.Helper()

	// Metrics provider
	metricsCollector := metricsmock.NewCollector("foo")
	metricsProvider := metricsmock.NewMetricsProvider()
	metricsProvider.RegisterConcreteCollector(provider.NewRuntimeMetadata(string(provider.RuntimeNameContainerd), ""), metricsCollector)
	metricsProvider.RegisterConcreteCollector(provider.NewRuntimeMetadata(string(provider.RuntimeNameGarden), ""), metricsCollector)

	// Workload meta + tagger
	metadataProvider := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	fakeTagger := taggerimpl.SetupFakeTagger(t)

	// Finally, container provider
	filter, err := containers.GetPauseContainerFilter()
	assert.NoError(t, err)
	return proccontainers.NewContainerProvider(metricsProvider, metadataProvider, filter, fakeTagger)
}

func TestProcessCheckFirstRun(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	proc2 := makeProcessWithCreateTime(2, "mine-bitcoins -all -x", now+1)
	proc3 := makeProcessWithCreateTime(3, "foo --version", now+2)
	proc4 := makeProcessWithCreateTime(4, "foo -bar -bim", now+3)
	proc5 := makeProcessWithCreateTime(5, "datadog-process-agent --cfgpath datadog.conf", now+2)
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}

	probe.On("ProcessesByPID", mock.Anything, mock.Anything).
		Return(processesByPid, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	expected := CombinedRunResult{}

	actual, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestProcessCheckSecondRun(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	proc2 := makeProcessWithCreateTime(2, "mine-bitcoins -all -x", now+1)
	proc3 := makeProcessWithCreateTime(3, "foo --version", now+2)
	proc4 := makeProcessWithCreateTime(4, "foo -bar -bim", now+3)
	proc5 := makeProcessWithCreateTime(5, "datadog-process-agent --cfgpath datadog.conf", now+2)

	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}

	probe.On("ProcessesByPID", mock.Anything, mock.Anything).
		Return(processesByPid, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	first, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	expected := []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc1, []string{"process_context:git"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc2, []string{"process_context:mine-bitcoins"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc3, []string{"process_context:foo"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc4, []string{"process_context:foo"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc5, []string{"process_context:datadog-process-agent"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
	}
	actual, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, actual.Payloads()) // ordering is not guaranteed
	assert.Nil(t, actual.RealtimePayloads())
}

func TestProcessCheckChunking(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		noChunking            bool
		expectedPayloadLength int
	}{
		{
			name:                  "Chunking",
			noChunking:            false,
			expectedPayloadLength: 5,
		},
		{
			name:                  "No chunking",
			noChunking:            true,
			expectedPayloadLength: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			processCheck, probe := processCheckWithMockProbe(t)

			// Set small chunk size to force chunking behavior
			processCheck.maxBatchBytes = 0
			processCheck.maxBatchSize = 0

			// mock processes
			now := time.Now().Unix()
			proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
			proc2 := makeProcessWithCreateTime(2, "mine-bitcoins -all -x", now+1)
			proc3 := makeProcessWithCreateTime(3, "foo --version", now+2)
			proc4 := makeProcessWithCreateTime(4, "foo -bar -bim", now+3)
			proc5 := makeProcessWithCreateTime(5, "datadog-process-agent --cfgpath datadog.conf", now+2)

			processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
			probe.On("ProcessesByPID", mock.Anything, mock.Anything).
				Return(processesByPid, nil)

			// Test second check runs without error and has correct number of chunks
			processCheck.Run(testGroupID(0), getChunkingOption(tc.noChunking))
			actual, err := processCheck.Run(testGroupID(0), getChunkingOption(tc.noChunking))
			require.NoError(t, err)
			assert.Len(t, actual.Payloads(), tc.expectedPayloadLength)
		})
	}
}

func TestProcessCheckWithRealtime(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)
	proc1 := makeProcess(1, "git clone google.com")
	proc2 := makeProcess(2, "mine-bitcoins -all -x")
	proc3 := makeProcess(3, "foo --version")
	proc4 := makeProcess(4, "foo -bar -bim")
	proc5 := makeProcess(5, "datadog-process-agent --cfgpath datadog.conf")

	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}

	probe.On("ProcessesByPID", mock.Anything, mock.Anything).
		Return(processesByPid, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	first, err := processCheck.run(0, true)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	expectedProcs := []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc1, []string{"process_context:git"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc2, []string{"process_context:mine-bitcoins"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc3, []string{"process_context:foo"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc4, []string{"process_context:foo"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc5, []string{"process_context:datadog-process-agent"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
	}

	expectedStats := makeProcessStatModels(t, proc1, proc2, proc3, proc4, proc5)
	actual, err := processCheck.run(0, true)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedProcs, actual.Payloads()) // ordering is not guaranteed
	require.Len(t, actual.RealtimePayloads(), 1)
	rt := actual.RealtimePayloads()[0].(*model.CollectorRealTime)
	assert.ElementsMatch(t, expectedStats, rt.Stats)
	assert.Equal(t, int32(1), rt.GroupSize)
	assert.Equal(t, int32(len(processCheck.hostInfo.SystemInfo.Cpus)), rt.NumCpus)
}

func TestOnlyEnvConfigArgsScrubbingEnabled(t *testing.T) {
	cfg := configmock.New(t)

	t.Setenv("DD_CUSTOM_SENSITIVE_WORDS", "*password*,consul_token,*api_key")

	scrubber := procutil.NewDefaultDataScrubber()
	initScrubber(cfg, scrubber)

	assert.True(t, scrubber.Enabled)

	cases := []struct {
		cmdline       []string
		parsedCmdline []string
	}{
		{
			[]string{"spidly", "--mypasswords=123,456", "consul_token", "1234", "--dd_api_key=1234"},
			[]string{"spidly", "--mypasswords=********", "consul_token", "********", "--dd_api_key=********"},
		},
	}

	for i := range cases {
		cases[i].cmdline, _ = scrubber.ScrubCommand(cases[i].cmdline)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestOnlyEnvConfigArgsScrubbingDisabled(t *testing.T) {
	cfg := configmock.New(t)

	t.Setenv("DD_SCRUB_ARGS", "false")
	t.Setenv("DD_CUSTOM_SENSITIVE_WORDS", "*password*,consul_token,*api_key")

	scrubber := procutil.NewDefaultDataScrubber()
	initScrubber(cfg, scrubber)

	assert.False(t, scrubber.Enabled)

	cases := []struct {
		cmdline       []string
		parsedCmdline []string
	}{
		{
			[]string{"spidly", "--mypasswords=123,456", "consul_token", "1234", "--dd_api_key=1234"},
			[]string{"spidly", "--mypasswords=123,456", "consul_token", "1234", "--dd_api_key=1234"},
		},
	}

	for i := range cases {
		fp := &procutil.Process{Cmdline: cases[i].cmdline}
		cases[i].cmdline = scrubber.ScrubProcessCommand(fp)
		assert.Equal(t, cases[i].parsedCmdline, cases[i].cmdline)
	}
}

func TestDisallowList(t *testing.T) {
	testDisallowList := []string{
		"^getty",
		"^acpid",
		"^atd",
		"^upstart-udev-bridge",
		"^upstart-socket-bridge",
		"^upstart-file-bridge",
		"^dhclient",
		"^dhclient3",
		"^rpc",
		"^dbus-daemon",
		"udevd",
		"^/sbin/",
		"^/usr/sbin/",
		"^/var/ossec/bin/ossec",
		"^rsyslogd",
		"^whoopsie$",
		"^cron$",
		"^CRON$",
		"^/usr/lib/postfix/master$",
		"^qmgr",
		"^pickup",
		"^sleep",
		"^/lib/systemd/systemd-logind$",
		"^/usr/local/bin/goshe dnsmasq$",
	}
	disallowList := make([]*regexp.Regexp, 0, len(testDisallowList))
	for _, b := range testDisallowList {
		r, err := regexp.Compile(b)
		if err == nil {
			disallowList = append(disallowList, r)
		}
	}
	cases := []struct {
		cmdline        []string
		disallowListed bool
	}{
		{[]string{"getty", "-foo", "-bar"}, true},
		{[]string{"rpcbind", "-x"}, true},
		{[]string{"my-rpc-app", "-config foo.ini"}, false},
		{[]string{"rpc.statd", "-L"}, true},
		{[]string{"/usr/sbin/irqbalance"}, true},
	}

	for _, c := range cases {
		assert.Equal(t, c.disallowListed, isDisallowListed(c.cmdline, disallowList),
			fmt.Sprintf("Case %v failed", c))
	}
}

func TestConnRates(t *testing.T) {
	p := &ProcessCheck{}

	p.initConnRates()

	var transmitter subscriptions.Transmitter[ProcessConnRates]
	transmitter.Chs = append(transmitter.Chs, p.connRatesReceiver.Ch)

	rates := ProcessConnRates{
		1: &model.ProcessNetworks{},
	}
	transmitter.Notify(rates)

	close(p.connRatesReceiver.Ch)

	assert.Eventually(t, func() bool { return p.getLastConnRates() != nil }, 10*time.Second, time.Millisecond)
	assert.Equal(t, rates, p.getLastConnRates())
}

func TestProcessCheckHints(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	processesByPid := map[int32]*procutil.Process{1: proc1}

	probe.On("ProcessesByPID", mock.Anything, mock.Anything).
		Return(processesByPid, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	first, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	expected := []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc1, []string{"process_context:git"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
	}
	actual, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, actual.Payloads()) // ordering is not guaranteed
	assert.Nil(t, actual.RealtimePayloads())

	expectedUnspecified := []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc1, []string{"process_context:git"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0},
		},
	}

	actual, err = processCheck.run(0, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedUnspecified, actual.Payloads()) // ordering is not guaranteed
	assert.Nil(t, actual.RealtimePayloads())

	expectedDiscovery := []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc1, []string{"process_context:git"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
	}

	actual, err = processCheck.run(0, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedDiscovery, actual.Payloads()) // ordering is not guaranteed
}

func TestProcessWithNoCommandline(t *testing.T) {
	var procMap = map[int32]*procutil.Process{
		1: makeProcess(1, ""),
	}
	procMap[1].Cmdline = nil
	procMap[1].Exe = "datadog-process-agent --cfgpath datadog.conf"

	lastRun := time.Now().Add(-5 * time.Second)
	syst1, syst2 := cpu.TimesStat{}, cpu.TimesStat{}

	var disallowList []*regexp.Regexp
	serviceExtractorEnabled := true
	useWindowsServiceName := true
	useImprovedAlgorithm := false
	serviceExtractor := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	procs := fmtProcesses(procutil.NewDefaultDataScrubber(), disallowList, procMap, procMap, nil, syst2, syst1, lastRun, nil, nil, false, serviceExtractor)
	assert.Len(t, procs, 1)

	require.Len(t, procs[""], 1)
	proc := procs[""][0]
	assert.Equal(t, procMap[1].Exe, proc.Command.Exe)
	assert.Empty(t, proc.Command.Args)
}

func BenchmarkProcessCheck(b *testing.B) {
	processCheck, probe := processCheckWithMockProbe(&testing.T{})

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	proc2 := makeProcessWithCreateTime(2, "mine-bitcoins -all -x", now+1)
	proc3 := makeProcessWithCreateTime(3, "foo --version", now+2)
	proc4 := makeProcessWithCreateTime(4, "foo -bar -bim", now+3)
	proc5 := makeProcessWithCreateTime(5, "datadog-process-agent --cfgpath datadog.conf", now+2)
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}

	probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(processesByPid, nil)

	for n := 0; n < b.N; n++ {
		_, err := processCheck.run(0, false)
		require.NoError(b, err)
	}
}

func TestProcessCheckZombieToggleFalse(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)
	cfg := configmock.New(t)
	processCheck.config = cfg
	processCheck.ignoreZombieProcesses = processCheck.config.GetBool(configIgnoreZombies)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	proc2 := makeProcessWithCreateTime(2, "foo -bar -bim", now+1)
	proc3 := makeProcessWithCreateTime(3, "datadog-process-agent --cfgpath datadog.conf", now+2)
	proc2.Stats.Status = "Z"
	proc3.Stats.Status = "Z"
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3}
	expectedModel2 := makeProcessModel(t, proc2, []string{"process_context:foo"})
	expectedModel2.State = 7
	expectedModel3 := makeProcessModel(t, proc3, []string{"process_context:datadog-process-agent"})
	expectedModel3.State = 7

	probe.On("ProcessesByPID", mock.Anything, mock.Anything).
		Return(processesByPid, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	first, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	expected := []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc1, []string{"process_context:git"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
		&model.CollectorProc{
			Processes: []*model.Process{expectedModel2},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
		&model.CollectorProc{
			Processes: []*model.Process{expectedModel3},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
	}
	actual, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, actual.Payloads())
}

func TestProcessCheckZombieToggleTrue(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)
	cfg := configmock.New(t)
	processCheck.config = cfg
	processCheck.ignoreZombieProcesses = processCheck.config.GetBool(configIgnoreZombies)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	proc2 := makeProcessWithCreateTime(2, "foo -bar -bim", now+1)
	proc3 := makeProcessWithCreateTime(3, "datadog-process-agent --cfgpath datadog.conf", now+2)
	proc2.Stats.Status = "Z"
	proc3.Stats.Status = "Z"
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3}

	probe.On("ProcessesByPID", mock.Anything, mock.Anything).
		Return(processesByPid, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	first, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	cfg.SetWithoutSource("process_config.ignore_zombie_processes", "true")
	processCheck.ignoreZombieProcesses = processCheck.config.GetBool(configIgnoreZombies)
	expected := []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc1, []string{"process_context:git"})},
			GroupSize: int32(1),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
	}

	actual, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, actual.Payloads()) // ordering is not guaranteed
}

func TestProcessContextCollection(t *testing.T) {
	processCheck, probe := processCheckWithMockProbe(t)
	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "/bin/bash/usr/local/bin/cilium-agent-bpf-map-metrics.sh", now)
	processesByPid := map[int32]*procutil.Process{1: proc1}
	probe.On("ProcessesByPID", mock.Anything, mock.Anything).
		Return(processesByPid, nil)
	first, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	expected := []model.MessageBody{
		&model.CollectorProc{
			Processes: []*model.Process{makeProcessModel(t, proc1, []string{"process_context:cilium-agent-bpf-map-metrics"})},
			GroupSize: int32(len(processesByPid)),
			Info:      processCheck.hostInfo.SystemInfo,
			Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
		},
	}

	actual, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, expected, actual.Payloads())
}
