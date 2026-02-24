// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/benbjohnson/clock"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	gpusubscriberfxmock "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/process/metadata"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	metricsmock "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// processCheckWithMocks returns a ProcessCheck along with the mocked probe and workloadmeta component.
func processCheckWithMocks(t *testing.T) (*ProcessCheck, *mocks.Probe, workloadmetamock.Mock) {
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

	mockGpuSubscriber := gpusubscriberfxmock.SetupMockGpuSubscriber(t)

	mockWLM := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		hostnameimpl.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

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
		gpuSubscriber:     mockGpuSubscriber,
		statsd:            &statsd.NoOpClient{},
		clock:             clock.NewMock(),
		wmeta:             mockWLM,
	}, probe, mockWLM
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
		hostnameimpl.MockModule(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	// Workload filter
	filterStore := workloadfilterfxmock.SetupMockFilter(t)
	filter := filterStore.GetContainerPausedFilters()

	// Finally, container provider
	return proccontainers.NewContainerProvider(metricsProvider, metadataProvider, filter, fakeTagger)
}

func procToWLMProc(proc *procutil.Process) *workloadmeta.Process {
	return &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(proc.Pid)),
		},
		Name:           proc.Name,
		Pid:            proc.Pid,
		Ppid:           proc.Ppid,
		NsPid:          proc.NsPid,
		Cwd:            proc.Cwd,
		Exe:            proc.Exe,
		Comm:           proc.Comm,
		Cmdline:        proc.Cmdline,
		Uids:           proc.Uids,
		Gids:           proc.Gids,
		CreationTime:   time.UnixMilli(proc.Stats.CreateTime),
		Language:       proc.Language,
		InjectionState: workloadmeta.InjectionState(proc.InjectionState),
	}
}

// mockProcesses sets up process mocks for either WLM or probe-based collection
func mockProcesses(wlmEnabled bool, probe *mocks.Probe, wmeta workloadmetamock.Mock, processesByPid map[int32]*procutil.Process, statsByPid map[int32]*procutil.Stats) {
	if wlmEnabled {
		for _, p := range processesByPid {
			wmeta.Set(procToWLMProc(p))
		}
		probe.On("StatsForPIDs", mock.Anything, mock.Anything).Return(statsByPid, nil)
	} else {
		probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(processesByPid, nil)
	}
}

func TestProcessCheckFirstRunWithProbe(t *testing.T) {
	processCheck, probe, wmeta := processCheckWithMocks(t)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	proc2 := makeProcessWithCreateTime(2, "mine-bitcoins -all -x", now+1)
	proc3 := makeProcessWithCreateTime(3, "foo --version", now+2)
	proc4 := makeProcessWithCreateTime(4, "foo -bar -bim", now+3)
	proc5 := makeProcessWithCreateTime(5, "datadog-process-agent --cfgpath datadog.conf", now+2)
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
	statsByPid := map[int32]*procutil.Stats{
		1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats, 4: proc4.Stats, 5: proc5.Stats,
	}

	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)

	// The first run returns nothing because processes must be observed on two consecutive runs
	expected := CombinedRunResult{}

	actual, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestProcessCheckSecondRunWithProbe(t *testing.T) {
	processCheck, probe, wmeta := processCheckWithMocks(t)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	proc2 := makeProcessWithCreateTime(2, "mine-bitcoins -all -x", now+1)
	proc3 := makeProcessWithCreateTime(3, "foo --version", now+2)
	proc4 := makeProcessWithCreateTime(4, "foo -bar -bim", now+3)
	proc5 := makeProcessWithCreateTime(5, "datadog-process-agent --cfgpath datadog.conf", now+2)

	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
	statsByPid := map[int32]*procutil.Stats{
		1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats, 4: proc4.Stats, 5: proc5.Stats,
	}

	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)

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
			processCheck, probe, wmeta := processCheckWithMocks(t)

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
			statsByPid := map[int32]*procutil.Stats{
				1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats, 4: proc4.Stats, 5: proc5.Stats,
			}
			mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)

			// Test second check runs without error and has correct number of chunks
			processCheck.Run(testGroupID(0), getChunkingOption(tc.noChunking))
			actual, err := processCheck.Run(testGroupID(0), getChunkingOption(tc.noChunking))
			require.NoError(t, err)
			assert.Len(t, actual.Payloads(), tc.expectedPayloadLength)
		})
	}
}

func TestProcessCheckEmpty(t *testing.T) {
	processCheck, probe, wmeta := processCheckWithMocks(t)

	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, nil, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	first, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	actual, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, actual)
}

func TestProcessCheckEmptyWithRealtime(t *testing.T) {
	processCheck, probe, wmeta := processCheckWithMocks(t)

	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, nil, nil)

	// The first run returns nothing because processes must be observed on two consecutive runs
	first, err := processCheck.run(0, true)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	actual, err := processCheck.run(0, true)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, actual)
}

func TestProcessCheckWithRealtime(t *testing.T) {
	processCheck, probe, wmeta := processCheckWithMocks(t)
	proc1 := makeProcess(1, "git clone google.com")
	proc2 := makeProcess(2, "mine-bitcoins -all -x")
	proc3 := makeProcess(3, "foo --version")
	proc4 := makeProcess(4, "foo -bar -bim")
	proc5 := makeProcess(5, "datadog-process-agent --cfgpath datadog.conf")

	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
	statsByPid := map[int32]*procutil.Stats{
		1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats, 4: proc4.Stats, 5: proc5.Stats,
	}

	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)

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

func TestProcessCheckHints(t *testing.T) {
	processCheck, probe, wmeta := processCheckWithMocks(t)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	processesByPid := map[int32]*procutil.Process{1: proc1}
	statsByPid := map[int32]*procutil.Stats{1: proc1.Stats}

	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)

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

	now := time.Now()
	lastRun := time.Now().Add(-5 * time.Second)
	syst1, syst2 := cpu.TimesStat{}, cpu.TimesStat{}

	var disallowList []*regexp.Regexp
	serviceExtractorEnabled := true
	useWindowsServiceName := true
	useImprovedAlgorithm := false
	serviceExtractor := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
	taggerMock := fxutil.Test[taggermock.Mock](t, core.MockBundle(), hostnameimpl.MockModule(), taggerfxmock.MockModule(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
	procs := fmtProcesses(procutil.NewDefaultDataScrubber(), disallowList, procMap, procMap, nil, syst2, syst1, lastRun, nil, false, serviceExtractor, nil, taggerMock, now)
	assert.Len(t, procs, 1)

	require.Len(t, procs[""], 1)
	proc := procs[""][0]
	assert.Equal(t, procMap[1].Exe, proc.Command.Exe)
	assert.Empty(t, proc.Command.Args)
}

func BenchmarkProcessCheck(b *testing.B) {
	testingT := &testing.T{}
	processCheck, probe, wmeta := processCheckWithMocks(testingT)

	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
	proc2 := makeProcessWithCreateTime(2, "mine-bitcoins -all -x", now+1)
	proc3 := makeProcessWithCreateTime(3, "foo --version", now+2)
	proc4 := makeProcessWithCreateTime(4, "foo -bar -bim", now+3)
	proc5 := makeProcessWithCreateTime(5, "datadog-process-agent --cfgpath datadog.conf", now+2)
	processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
	statsByPid := map[int32]*procutil.Stats{
		1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats, 4: proc4.Stats, 5: proc5.Stats,
	}

	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)

	for n := 0; n < b.N; n++ {
		_, err := processCheck.run(0, false)
		require.NoError(b, err)
	}
}

func TestProcessCheckZombieToggleFalse(t *testing.T) {
	processCheck, probe, wmeta := processCheckWithMocks(t)
	cfg := configmock.New(t)
	processCheck.config = cfg
	cfg.SetInTest("process_config.ignore_zombie_processes", false)
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

	statsByPid := map[int32]*procutil.Stats{1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats}

	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)

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
	processCheck, probe, wmeta := processCheckWithMocks(t)
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
	statsByPid := map[int32]*procutil.Stats{1: proc1.Stats, 2: proc2.Stats, 3: proc3.Stats}

	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)

	// The first run returns nothing because processes must be observed on two consecutive runs
	first, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	cfg.SetInTest("process_config.ignore_zombie_processes", "true")
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
	processCheck, probe, wmeta := processCheckWithMocks(t)
	now := time.Now().Unix()
	proc1 := makeProcessWithCreateTime(1, "/bin/bash/usr/local/bin/cilium-agent-bpf-map-metrics.sh", now)
	processesByPid := map[int32]*procutil.Process{1: proc1}
	statsByPid := map[int32]*procutil.Stats{1: proc1.Stats}
	mockProcesses(processCheck.WLMProcessCollectionEnabled(), probe, wmeta, processesByPid, statsByPid)
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

func TestProcessTaggerIntegration(t *testing.T) {
	now := time.Now()
	lastRun := now.Add(-5 * time.Second)
	syst1, syst2 := cpu.TimesStat{}, cpu.TimesStat{}

	// Create test processes using the proper helper function
	proc1 := makeProcessWithCreateTime(1234, "test-process --config server.conf", now.Unix())
	proc2 := makeProcessWithCreateTime(5678, "another-process --worker", now.Unix())
	proc3 := makeProcessWithCreateTime(9101, "yet-another-process --worker", now.Unix())

	procs := map[int32]*procutil.Process{
		1234: proc1,
		5678: proc2,
		9101: proc3,
	}

	// Create tagger mock and configure it to return specific tags for our test processes
	taggerMock := fxutil.Test[taggermock.Mock](t, core.MockBundle(), hostnameimpl.MockModule(), taggerfxmock.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()))

	// Set up expected tags for process 1234
	proc1Tags := []string{"service:web-server", "env:production", "team:backend"}
	taggerMock.SetTags(taggertypes.NewEntityID(taggertypes.Process, "1234"), "test-source", nil, nil, proc1Tags, nil)

	// Set up expected tags for process 5678 (empty tags to test that case)
	taggerMock.SetTags(taggertypes.NewEntityID(taggertypes.Process, "5678"), "test-source", nil, nil, []string{}, nil)

	// Create service extractor
	serviceExtractorEnabled := true
	useWindowsServiceName := false
	useImprovedAlgorithm := false
	serviceExtractor := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)

	// Call fmtProcesses with the tagger
	procsByCtr := fmtProcesses(
		procutil.NewDefaultDataScrubber(),
		nil, // no disallow list
		procs,
		procs, // same as last procs for simplicity
		nil,   // no container mapping
		syst2,
		syst1,
		lastRun,
		nil,   // no lookup probe
		false, // don't ignore zombies
		serviceExtractor,
		nil, // no GPU tags
		taggerMock,
		now,
	)

	// Verify that we have processes in the result
	require.Contains(t, procsByCtr, "", "Expected non-container processes")
	processes := procsByCtr[""]
	require.Len(t, processes, 3, "Expected 3 processes")

	// Find the processes by PID and verify their tags
	var proc1234, proc5678, proc9101 *model.Process
	for _, p := range processes {
		switch p.Pid {
		case 1234:
			proc1234 = p
		case 5678:
			proc5678 = p
		case 9101:
			proc9101 = p
		}
	}

	require.NotNil(t, proc1234, "Process 1234 should be found")
	require.NotNil(t, proc5678, "Process 5678 should be found")
	require.NotNil(t, proc9101, "Process 9101 should be found")

	// Verify that process 1234 has the expected tags from tagger
	assert.Contains(t, proc1234.Tags, "service:web-server", "Process 1234 should have service tag from tagger")
	assert.Contains(t, proc1234.Tags, "env:production", "Process 1234 should have env tag from tagger")
	assert.Contains(t, proc1234.Tags, "team:backend", "Process 1234 should have team tag from tagger")

	// Verify that process 5678 has no tags (empty tags from tagger)
	assert.Empty(t, proc5678.Tags, "Process 5678 should have no tags")

	// Verify that process 9101 has no tags (no tags in tagger)
	assert.Empty(t, proc9101.Tags, "Process 9101 should have no tags")
}
