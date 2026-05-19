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
	procs := fmtProcesses(procutil.NewDefaultDataScrubber(), disallowList, procMap, procMap, nil, syst2, syst1, lastRun, nil, nil, serviceExtractor, nil, taggerMock, now)
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
		nil, // no lookup probe
		nil, // no zombie aggregates
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

// zombieProc builds a zombie procutil.Process for aggregator tests.
func zombieProc(pid, ppid int32) *procutil.Process {
	return &procutil.Process{
		Pid:   pid,
		Ppid:  ppid,
		Stats: &procutil.Stats{Status: "Z"},
	}
}

// liveProc builds a non-zombie procutil.Process for aggregator tests.
func liveProc(pid, ppid int32) *procutil.Process {
	return &procutil.Process{
		Pid:   pid,
		Ppid:  ppid,
		Stats: &procutil.Stats{Status: "S"},
	}
}

func TestAggregateZombiesByParent(t *testing.T) {
	const intervalSec = 10
	lastRun := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := lastRun.Add(intervalSec * time.Second)

	cases := []struct {
		name      string
		lastProcs map[int32]*procutil.Process
		procs     map[int32]*procutil.Process
		lastRun   time.Time
		want      map[int32]zombieAggregate
	}{
		{
			name: "active leak: 4 new zombies, none reaped",
			lastProcs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				200: zombieProc(200, 100),
			},
			procs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				200: zombieProc(200, 100),
				201: zombieProc(201, 100),
				202: zombieProc(202, 100),
				203: zombieProc(203, 100),
				204: zombieProc(204, 100),
			},
			lastRun: lastRun,
			want: map[int32]zombieAggregate{
				100: {count: 5, netRate: 4.0 / intervalSec},
			},
		},
		{
			name: "draining: 4 zombies reaped, none new",
			lastProcs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				200: zombieProc(200, 100),
				201: zombieProc(201, 100),
				202: zombieProc(202, 100),
				203: zombieProc(203, 100),
				204: zombieProc(204, 100),
			},
			procs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				200: zombieProc(200, 100),
			},
			lastRun: lastRun,
			want: map[int32]zombieAggregate{
				100: {count: 1, netRate: -4.0 / intervalSec},
			},
		},
		{
			name: "stable busy: full churn nets zero rate",
			lastProcs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				200: zombieProc(200, 100),
				201: zombieProc(201, 100),
				202: zombieProc(202, 100),
			},
			procs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				300: zombieProc(300, 100),
				301: zombieProc(301, 100),
				302: zombieProc(302, 100),
			},
			lastRun: lastRun,
			want: map[int32]zombieAggregate{
				100: {count: 3, netRate: 0},
			},
		},
		{
			name: "stable dormant: no zombies returns nil map",
			lastProcs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				101: liveProc(101, 1),
			},
			procs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				101: liveProc(101, 1),
			},
			lastRun: lastRun,
			want:    nil,
		},
		{
			name:      "first poll clamps rate to zero",
			lastProcs: nil,
			procs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				200: zombieProc(200, 100),
				201: zombieProc(201, 100),
				202: zombieProc(202, 100),
			},
			lastRun: time.Time{},
			want: map[int32]zombieAggregate{
				100: {count: 3, netRate: 0},
			},
		},
		{
			name: "reaped zombie debits previous parent",
			lastProcs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				101: liveProc(101, 1),
				200: zombieProc(200, 100),
			},
			procs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				101: liveProc(101, 1),
			},
			lastRun: lastRun,
			want: map[int32]zombieAggregate{
				100: {count: 0, netRate: -1.0 / intervalSec},
			},
		},
		{
			name: "reparented persistent zombie: count only, no rate movement",
			lastProcs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				101: liveProc(101, 1),
				200: zombieProc(200, 100),
			},
			procs: map[int32]*procutil.Process{
				100: liveProc(100, 1),
				101: liveProc(101, 1),
				200: zombieProc(200, 101),
			},
			lastRun: lastRun,
			want: map[int32]zombieAggregate{
				101: {count: 1, netRate: 0},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &ProcessCheck{lastProcs: tc.lastProcs, lastRun: tc.lastRun}
			got := p.aggregateZombiesByParent(tc.procs, now)
			if tc.want == nil {
				assert.Nil(t, got, "expected nil map for no-zombie case (allocation-free path)")
				return
			}
			require.NotNil(t, got, "expected non-nil aggregate map")
			require.Len(t, got, len(tc.want))
			for ppid, wantAgg := range tc.want {
				gotAgg, ok := got[ppid]
				require.Truef(t, ok, "missing PPID %d in result", ppid)
				assert.Equalf(t, wantAgg.count, gotAgg.count, "count mismatch for PPID %d", ppid)
				assert.InDeltaf(t, wantAgg.netRate, gotAgg.netRate, 1e-9, "netRate mismatch for PPID %d", ppid)
			}
		})
	}
}

// TestAggregateZombiesByParent_NilStats confirms that procs with nil Stats
// (which can happen for transient probe edge cases) are skipped without
// panicking, in both current and previous maps.
func TestAggregateZombiesByParent_NilStats(t *testing.T) {
	lastRun := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	now := lastRun.Add(10 * time.Second)

	lastProcs := map[int32]*procutil.Process{
		100: liveProc(100, 1),
		201: {Pid: 201, Ppid: 100, Stats: nil}, // previous: nil stats — must not be counted as zombie
	}
	procs := map[int32]*procutil.Process{
		100: liveProc(100, 1),
		200: {Pid: 200, Ppid: 100, Stats: nil}, // current: nil stats
		201: zombieProc(201, 100),
	}

	p := &ProcessCheck{lastProcs: lastProcs, lastRun: lastRun}
	got := p.aggregateZombiesByParent(procs, now)
	require.NotNil(t, got)
	require.Contains(t, got, int32(100))
	// Only pid=201 is a real zombie in current; pid=200 has nil Stats so it's
	// skipped. pid=201 was nil in lastProcs so it counts as "new" under PPID=100.
	assert.Equal(t, uint32(1), got[100].count)
	assert.InDelta(t, 1.0/10.0, got[100].netRate, 1e-9)
}

// makeZombieProcessWithPpid builds a fully-formed procutil.Process suitable for
// the run-level zombie-aggregation E2E test: it has real Stats (MemInfo,
// CPUPercent, IOStat) so fmtProcesses can format it, plus Status="Z" and the
// requested Ppid. Used both as a zombie that should be skipped from the output
// and to seed lastProcs.
func makeZombieProcessWithPpid(pid, ppid int32, createTime int64) *procutil.Process {
	p := makeProcessWithCreateTime(pid, fmt.Sprintf("zombie-%d", pid), createTime)
	p.Ppid = ppid
	p.Stats.Status = "Z"
	return p
}

// makeLiveProcessWithPpid is the live-parent counterpart to
// makeZombieProcessWithPpid: a fully-formed procutil.Process with Status="S".
func makeLiveProcessWithPpid(pid, ppid int32, cmdline string, createTime int64) *procutil.Process {
	p := makeProcessWithCreateTime(pid, cmdline, createTime)
	p.Ppid = ppid
	p.Stats.Status = "S"
	return p
}

// TestProcessCheckRunZombieAggregation is the run-level E2E assertion that
// CXP-3539's wire-up is correct: across two consecutive run() polls,
// (a) zombie processes never surface as standalone records in Payloads(), and
// (b) the parent's record carries the correct ZombieChildrenCount and
// ZombieNetRate computed by aggregateZombiesByParent.
//
// Setup:
//
//	Poll 1: parent (pid=100, alive), Z1 (pid=200, ppid=100), Z2 (pid=201, ppid=100)
//	Poll 2: parent still alive, Z1 reaped (absent), Z2 still zombie, Z3 (pid=202, ppid=100) new
//
// With a 10s interval between polls: count=2 (Z2, Z3), netRate=(1 new - 1 reap)/10 = 0.
// Both polls share an unrelated bystander process (pid=300) to confirm parents
// without zombie children get the zero default (no extra allocations / no
// spurious fields).
func TestProcessCheckRunZombieAggregation(t *testing.T) {
	processCheck, probe, wmeta := processCheckWithMocks(t)

	createdAt := time.Now().Unix()

	// Poll 1 process set: parent + 2 zombies + 1 bystander.
	parent1 := makeLiveProcessWithPpid(100, 1, "leaky-app --serve", createdAt)
	z1Poll1 := makeZombieProcessWithPpid(200, 100, createdAt)
	z2Poll1 := makeZombieProcessWithPpid(201, 100, createdAt+1)
	bystander1 := makeLiveProcessWithPpid(300, 1, "unrelated --idle", createdAt)

	procsPoll1 := map[int32]*procutil.Process{
		100: parent1,
		200: z1Poll1,
		201: z2Poll1,
		300: bystander1,
	}
	statsPoll1 := map[int32]*procutil.Stats{
		100: parent1.Stats, 200: z1Poll1.Stats, 201: z2Poll1.Stats, 300: bystander1.Stats,
	}

	// Poll 2 process set: parent still alive, Z1 reaped (absent), Z2 still
	// zombie, Z3 new zombie. Bystander still alive.
	parent2 := makeLiveProcessWithPpid(100, 1, "leaky-app --serve", createdAt)
	z2Poll2 := makeZombieProcessWithPpid(201, 100, createdAt+1)
	z3Poll2 := makeZombieProcessWithPpid(202, 100, createdAt+2)
	bystander2 := makeLiveProcessWithPpid(300, 1, "unrelated --idle", createdAt)

	procsPoll2 := map[int32]*procutil.Process{
		100: parent2,
		201: z2Poll2,
		202: z3Poll2,
		300: bystander2,
	}
	statsPoll2 := map[int32]*procutil.Stats{
		100: parent2.Stats, 201: z2Poll2.Stats, 202: z3Poll2.Stats, 300: bystander2.Stats,
	}

	// Wire up mocks so each poll returns its own process set. The WLM path
	// reads wmeta.ListProcesses() then probe.StatsForPIDs; the non-WLM path
	// calls probe.ProcessesByPID directly. mock.Once() ensures the second
	// run() sees the second-poll data.
	if processCheck.WLMProcessCollectionEnabled() {
		for _, p := range procsPoll1 {
			wmeta.Set(procToWLMProc(p))
		}
		probe.On("StatsForPIDs", mock.Anything, mock.Anything).Return(statsPoll1, nil).Once()
		probe.On("StatsForPIDs", mock.Anything, mock.Anything).Return(statsPoll2, nil).Once()
	} else {
		probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(procsPoll1, nil).Once()
		probe.On("ProcessesByPID", mock.Anything, mock.Anything).Return(procsPoll2, nil).Once()
	}

	// First poll: primes lastProcs/lastRun. Returns empty.
	first, err := processCheck.run(0, false)
	require.NoError(t, err)
	assert.Equal(t, CombinedRunResult{}, first)

	// Update wmeta state for the second poll: unset reaped Z1, set new Z3.
	// (Z2, parent, and bystander remain set from poll 1.) The Stats mock above
	// already returns statsPoll2 on its second call.
	if processCheck.WLMProcessCollectionEnabled() {
		wmeta.Unset(procToWLMProc(z1Poll1))
		wmeta.Set(procToWLMProc(z3Poll2))
	}

	// Advance the mock clock so the next run() sees a non-zero interval and
	// netRate is exercised. 10 seconds gives 1/interval = 0.1 granularity.
	const intervalSec = 10
	processCheck.clock.(*clock.Mock).Add(intervalSec * time.Second)

	// Second poll: zombies must be absent from Payloads(); parent must carry
	// the aggregated zombie fields.
	actual, err := processCheck.run(0, false)
	require.NoError(t, err)
	payloads := actual.Payloads()
	require.NotEmpty(t, payloads, "expected non-empty payloads on second poll")

	// Collect all emitted processes across CollectorProc messages and index by
	// PID, asserting that no zombie ever surfaces.
	emittedByPid := map[int32]*model.Process{}
	for _, msg := range payloads {
		cp, ok := msg.(*model.CollectorProc)
		require.True(t, ok, "expected *model.CollectorProc payload, got %T", msg)
		for _, p := range cp.Processes {
			assert.NotEqualf(t, model.ProcessState(model.ProcessState_value["Z"]), p.State,
				"zombie pid=%d must not appear as a standalone record", p.Pid)
			_, dup := emittedByPid[p.Pid]
			require.Falsef(t, dup, "pid=%d emitted twice", p.Pid)
			emittedByPid[p.Pid] = p
		}
	}

	// Zombie PIDs must not be in the output, ever.
	for _, pid := range []int32{200, 201, 202} {
		_, ok := emittedByPid[pid]
		assert.Falsef(t, ok, "zombie pid=%d leaked into Payloads()", pid)
	}

	// Parent record must carry the aggregated zombie fields.
	parentRec, ok := emittedByPid[100]
	require.True(t, ok, "parent pid=100 missing from Payloads()")
	assert.Equal(t, uint32(2), parentRec.ZombieChildrenCount,
		"parent must report 2 zombie children (Z2 still-zombie, Z3 new)")
	// netRate = (1 new − 1 reaped) / 10s = 0.
	assert.InDelta(t, 0.0, parentRec.ZombieNetRate, 1e-9,
		"parent netRate must be 0: 1 new − 1 reaped in 10s")

	// Bystander parent (no zombie children) must get the zero default —
	// confirms parents without entries in the aggregate map receive zero
	// values rather than spurious data.
	bystanderRec, ok := emittedByPid[300]
	require.True(t, ok, "bystander pid=300 missing from Payloads()")
	assert.Equal(t, uint32(0), bystanderRec.ZombieChildrenCount,
		"bystander parent must have zero zombie children")
	assert.InDelta(t, 0.0, bystanderRec.ZombieNetRate, 1e-9,
		"bystander parent must have zero netRate")
}
