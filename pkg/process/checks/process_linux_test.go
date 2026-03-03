// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package checks

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/core"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/discovery/usm"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	probemocks "github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/fx"
)

// TestProcessesByPIDWLM tests processesByPID map creation when WLM collection is ON
func TestProcessesByPIDWLM(t *testing.T) {
	mockConstantClock := constantMockClock(time.Now())
	nowMs := mockConstantClock.Now().UnixMilli()
	// TEST DATA 1
	proc1 := wlmProcessWithCreateTime(1, "git clone google.com", nowMs)
	proc2 := wlmProcessWithCreateTime(2, "mine-bitcoins -all -x", nowMs-1)
	proc3 := wlmProcessWithCreateTime(3, "datadog-agent --cfgpath datadog.conf", nowMs+2)
	proc4 := wlmProcessWithServiceDiscovery(4, "/bin/bash/usr/local/bin/cilium-agent-bpf-map-metrics.sh", nowMs-3)
	wlmProcesses := []*wmdef.Process{proc1, proc2, proc3, proc4}
	statsByPid := createTestWLMProcessStats([]*wmdef.Process{proc1, proc2, proc3, proc4}, true)
	expected1 := map[int32]*procutil.Process{
		proc1.Pid: mapWLMProcToProc(proc1, statsByPid[proc1.Pid]),
		proc2.Pid: mapWLMProcToProc(proc2, statsByPid[proc2.Pid]),
		proc3.Pid: mapWLMProcToProc(proc3, statsByPid[proc3.Pid]),
		proc4.Pid: mapWLMProcToProc(proc4, statsByPid[proc4.Pid]),
	}

	// TEST DATA 2
	statsByPidMissingProc1 := createTestWLMProcessStats([]*wmdef.Process{proc2, proc3, proc4}, true)
	expected2 := map[int32]*procutil.Process{
		proc2.Pid: mapWLMProcToProc(proc2, statsByPidMissingProc1[proc2.Pid]),
		proc3.Pid: mapWLMProcToProc(proc3, statsByPidMissingProc1[proc3.Pid]),
		proc4.Pid: mapWLMProcToProc(proc4, statsByPidMissingProc1[proc4.Pid]),
	}

	// TEST DATA 3
	newProc1 := wlmProcessWithCreateTime(1, "git clone google.com", nowMs+10)
	statsByPidNewProc1 := createTestWLMProcessStats([]*wmdef.Process{newProc1, proc2, proc3, proc4}, true)
	expected3 := map[int32]*procutil.Process{
		proc2.Pid: mapWLMProcToProc(proc2, statsByPidNewProc1[proc2.Pid]),
		proc3.Pid: mapWLMProcToProc(proc3, statsByPidNewProc1[proc3.Pid]),
		proc4.Pid: mapWLMProcToProc(proc4, statsByPidNewProc1[proc4.Pid]),
	}

	for _, tc := range []struct {
		description  string
		wlmProcesses []*wmdef.Process
		statsByPid   map[int32]*procutil.Stats
		expected     map[int32]*procutil.Process
	}{
		{
			description:  "normal wlm collection",
			wlmProcesses: wlmProcesses,
			statsByPid:   statsByPid,
			expected:     expected1,
		},
		{
			description:  "race condition - process dies after wlm collection before stat collection",
			wlmProcesses: []*wmdef.Process{proc1, proc2, proc3, proc4},
			statsByPid:   statsByPidMissingProc1,
			expected:     expected2,
		},
		{
			description:  "race condition - process dies after wlm collection with new process and same PID before stat collection",
			wlmProcesses: []*wmdef.Process{proc1, proc2, proc3, proc4},
			statsByPid:   statsByPidNewProc1,
			expected:     expected3,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			// INITIALIZATION
			mockProbe := probemocks.NewProbe(t)
			mockWLM := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				workloadmetafxmock.MockModule(wmdef.NewParams()),
			))

			processCheck := &ProcessCheck{
				wmeta: mockWLM,
				probe: mockProbe,
				clock: mockConstantClock,
			}

			// MOCKING
			for _, p := range tc.wlmProcesses {
				mockWLM.Set(p)
			}

			// elevatedPermissions is irrelevant since we are mocking the probe so no internal logic is tested
			mockProbe.EXPECT().StatsForPIDs(mock.Anything, mockConstantClock.Now()).Return(tc.statsByPid, nil).Once()

			// TESTING
			actual, err := processCheck.processesByPID()
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestFormatPorts(t *testing.T) {
	for _, tc := range []struct {
		description      string
		portsCollected   bool
		tcpPorts         []uint16
		udpPorts         []uint16
		expectedPortInfo *model.PortInfo
	}{
		{
			description:    "normal tcp and udp ports",
			portsCollected: true,
			tcpPorts:       []uint16{80, 443},
			udpPorts:       []uint16{53, 123},
			expectedPortInfo: &model.PortInfo{
				Tcp: []int32{80, 443},
				Udp: []int32{53, 123},
			},
		},
		{
			description:    "tcp only ports",
			portsCollected: true,
			tcpPorts:       []uint16{80, 443},
			udpPorts:       nil,
			expectedPortInfo: &model.PortInfo{
				Tcp: []int32{80, 443},
				Udp: nil,
			},
		},
		{
			description:    "udp only ports",
			portsCollected: true,
			tcpPorts:       nil,
			udpPorts:       []uint16{53, 123},
			expectedPortInfo: &model.PortInfo{
				Tcp: nil,
				Udp: []int32{53, 123},
			},
		},
		{
			description:    "empty ports",
			portsCollected: true,
			tcpPorts:       []uint16{},
			udpPorts:       []uint16{},
			expectedPortInfo: &model.PortInfo{
				Tcp: []int32{},
				Udp: []int32{},
			},
		},
		{
			description:      "ports not collected",
			portsCollected:   false,
			tcpPorts:         []uint16{},
			udpPorts:         []uint16{},
			expectedPortInfo: nil,
		},
		{
			description:      "ports not collected",
			portsCollected:   false,
			tcpPorts:         nil,
			udpPorts:         nil,
			expectedPortInfo: nil,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			actual := formatPorts(tc.portsCollected, tc.tcpPorts, tc.udpPorts)
			assert.Equal(t, tc.expectedPortInfo, actual)
		})
	}
}

func TestFormatLanguage(t *testing.T) {
	for _, tc := range []struct {
		description      string
		language         *languagemodels.Language
		expectedLanguage model.Language
	}{
		{
			description: "go",
			language: &languagemodels.Language{
				Name: languagemodels.Go,
			},
			expectedLanguage: model.Language_LANGUAGE_GO,
		},
		{
			description: "node",
			language: &languagemodels.Language{
				Name: languagemodels.Node,
			},
			expectedLanguage: model.Language_LANGUAGE_NODE,
		},
		{
			description: "dotnet",
			language: &languagemodels.Language{
				Name: languagemodels.Dotnet,
			},
			expectedLanguage: model.Language_LANGUAGE_DOTNET,
		},
		{
			description: "python",
			language: &languagemodels.Language{
				Name: languagemodels.Python,
			},
			expectedLanguage: model.Language_LANGUAGE_PYTHON,
		},
		{
			description: "java",
			language: &languagemodels.Language{
				Name: languagemodels.Java,
			},
			expectedLanguage: model.Language_LANGUAGE_JAVA,
		},
		{
			description: "ruby",
			language: &languagemodels.Language{
				Name: languagemodels.Ruby,
			},
			expectedLanguage: model.Language_LANGUAGE_RUBY,
		},
		{
			description: "php",
			language: &languagemodels.Language{
				Name: languagemodels.PHP,
			},
			expectedLanguage: model.Language_LANGUAGE_PHP,
		},
		{
			description: "cpp",
			language: &languagemodels.Language{
				Name: languagemodels.CPP,
			},
			expectedLanguage: model.Language_LANGUAGE_CPP,
		},
		{
			description: "unknown",
			language: &languagemodels.Language{
				Name: languagemodels.Unknown,
			},
			expectedLanguage: model.Language_LANGUAGE_UNKNOWN,
		},
		{
			description:      "not collected",
			language:         nil,
			expectedLanguage: model.Language_LANGUAGE_UNKNOWN,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			actual := formatLanguage(tc.language)
			assert.Equal(t, tc.expectedLanguage, actual)
		})
	}
}

func TestServiceNameSourceMap(t *testing.T) {
	for _, tc := range []struct {
		description               string
		serviceNameSource         string
		expectedServiceNameSource model.ServiceNameSource
	}{
		{
			description:               "command line",
			serviceNameSource:         string(usm.CommandLine),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_COMMAND_LINE,
		},
		{
			description:               "laravel",
			serviceNameSource:         string(usm.Laravel),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_LARAVEL,
		},
		{
			description:               "python",
			serviceNameSource:         string(usm.Python),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_PYTHON,
		},
		{
			description:               "nodejs",
			serviceNameSource:         string(usm.Nodejs),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_NODEJS,
		},
		{
			description:               "gunicorn",
			serviceNameSource:         string(usm.Gunicorn),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_GUNICORN,
		},
		{
			description:               "rails",
			serviceNameSource:         string(usm.Rails),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_RAILS,
		},
		{
			description:               "spring",
			serviceNameSource:         string(usm.Spring),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_SPRING,
		},
		{
			description:               "jboss",
			serviceNameSource:         string(usm.JBoss),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_JBOSS,
		},
		{
			description:               "tomcat",
			serviceNameSource:         string(usm.Tomcat),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_TOMCAT,
		},
		{
			description:               "weblogic",
			serviceNameSource:         string(usm.WebLogic),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_WEBLOGIC,
		},
		{
			description:               "websphere",
			serviceNameSource:         string(usm.WebSphere),
			expectedServiceNameSource: model.ServiceNameSource_SERVICE_NAME_SOURCE_WEBSPHERE,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			actual := serviceNameSource(tc.serviceNameSource)
			assert.Equal(t, tc.expectedServiceNameSource, actual)
		})
	}
}

func TestFormatServiceDiscovery(t *testing.T) {
	for _, tc := range []struct {
		description     string
		service         *procutil.Service
		expectedService *model.ServiceDiscovery
	}{
		{
			description: "complete service",
			service: &procutil.Service{
				GeneratedName:            "gen_name",
				GeneratedNameSource:      "unknown",
				AdditionalGeneratedNames: []string{"additional_name1", "additional_name2"},
				TracerMetadata: []tracermetadata.TracerMetadata{
					{
						RuntimeID:   "run_id1",
						ServiceName: "service_name1",
					},
					{
						RuntimeID:   "run_id2",
						ServiceName: "service_name2",
					},
				},
				DDService:          "dd_service_name",
				APMInstrumentation: true,
				LogFiles:           []string{"/var/log/app.log", "/var/log/error.log"},
			},
			expectedService: &model.ServiceDiscovery{
				GeneratedServiceName: &model.ServiceName{
					Name:   "gen_name",
					Source: model.ServiceNameSource_SERVICE_NAME_SOURCE_UNKNOWN,
				},
				DdServiceName: &model.ServiceName{
					Name:   "dd_service_name",
					Source: model.ServiceNameSource_SERVICE_NAME_SOURCE_DD_SERVICE,
				},
				AdditionalGeneratedNames: []*model.ServiceName{
					{
						Name:   "additional_name1",
						Source: model.ServiceNameSource_SERVICE_NAME_SOURCE_UNKNOWN,
					},
					{
						Name:   "additional_name2",
						Source: model.ServiceNameSource_SERVICE_NAME_SOURCE_UNKNOWN,
					},
				},
				TracerMetadata: []*model.TracerMetadata{
					{
						RuntimeId:   "run_id1",
						ServiceName: "service_name1",
					},
					{
						RuntimeId:   "run_id2",
						ServiceName: "service_name2",
					},
				},
				ApmInstrumentation: true,
				Resources: []*model.Resource{
					{
						Resource: &model.Resource_Logs{
							Logs: &model.LogResource{
								Path: "/var/log/app.log",
							},
						},
					},
					{
						Resource: &model.Resource_Logs{
							Logs: &model.LogResource{
								Path: "/var/log/error.log",
							},
						},
					},
				},
			},
		},
		{
			description: "empty service names",
			service: &procutil.Service{
				GeneratedName:            "",
				GeneratedNameSource:      "",
				AdditionalGeneratedNames: []string{"", ""},
				DDService:                "",
				APMInstrumentation:       false,
			},
			expectedService: &model.ServiceDiscovery{
				GeneratedServiceName:     nil,
				DdServiceName:            nil,
				AdditionalGeneratedNames: nil,
				ApmInstrumentation:       false,
			},
		},
		{
			description:     "empty service",
			service:         &procutil.Service{},
			expectedService: &model.ServiceDiscovery{},
		},
		{
			description: "service with log files only",
			service: &procutil.Service{
				LogFiles: []string{"/var/log/nginx/access.log", "/var/log/nginx/error.log", "/var/log/app/application.log"},
			},
			expectedService: &model.ServiceDiscovery{
				Resources: []*model.Resource{
					{
						Resource: &model.Resource_Logs{
							Logs: &model.LogResource{
								Path: "/var/log/nginx/access.log",
							},
						},
					},
					{
						Resource: &model.Resource_Logs{
							Logs: &model.LogResource{
								Path: "/var/log/nginx/error.log",
							},
						},
					},
					{
						Resource: &model.Resource_Logs{
							Logs: &model.LogResource{
								Path: "/var/log/app/application.log",
							},
						},
					},
				},
			},
		},
		{
			description: "service with single log file",
			service: &procutil.Service{
				GeneratedName: "my-service",
				LogFiles:      []string{"/var/log/service.log"},
			},
			expectedService: &model.ServiceDiscovery{
				GeneratedServiceName: &model.ServiceName{
					Name:   "my-service",
					Source: model.ServiceNameSource_SERVICE_NAME_SOURCE_UNKNOWN,
				},
				Resources: []*model.Resource{
					{
						Resource: &model.Resource_Logs{
							Logs: &model.LogResource{
								Path: "/var/log/service.log",
							},
						},
					},
				},
			},
		},
		{
			description: "service with nil log files",
			service: &procutil.Service{
				GeneratedName: "my-service",
				LogFiles:      nil,
			},
			expectedService: &model.ServiceDiscovery{
				GeneratedServiceName: &model.ServiceName{
					Name:   "my-service",
					Source: model.ServiceNameSource_SERVICE_NAME_SOURCE_UNKNOWN,
				},
				Resources: nil,
			},
		},
		{
			description: "service with empty log files slice",
			service: &procutil.Service{
				GeneratedName: "my-service",
				LogFiles:      []string{},
			},
			expectedService: &model.ServiceDiscovery{
				GeneratedServiceName: &model.ServiceName{
					Name:   "my-service",
					Source: model.ServiceNameSource_SERVICE_NAME_SOURCE_UNKNOWN,
				},
				Resources: nil,
			},
		},
		{
			description:     "service not collected",
			service:         nil,
			expectedService: nil,
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			actual := formatServiceDiscovery(tc.service)
			assert.Equal(t, tc.expectedService, actual)
		})
	}
}

func wlmProcessWithCreateTime(pid int32, spaceSeparatedCmdline string, creationTime int64) *wmdef.Process {
	return &wmdef.Process{
		EntityID: wmdef.EntityID{
			ID:   strconv.Itoa(int(pid)),
			Kind: wmdef.KindProcess,
		},
		Pid:          pid,
		Cmdline:      strings.Split(spaceSeparatedCmdline, " "),
		CreationTime: time.Unix(creationTime, 0),
	}
}

func wlmProcessWithServiceDiscovery(pid int32, spaceSeparatedCmdline string, creationTime int64) *wmdef.Process {
	proc := wlmProcessWithCreateTime(pid, spaceSeparatedCmdline, creationTime)
	proc.Service = &wmdef.Service{
		GeneratedName:            "some generated name",
		GeneratedNameSource:      string(usm.CommandLine),
		AdditionalGeneratedNames: []string{"some additional name", "another additional name"},
		TracerMetadata: []tracermetadata.TracerMetadata{
			{
				RuntimeID:   "some-runtime-id",
				ServiceName: "some-tracer-service",
			},
		},
		UST: wmdef.UST{
			Service: "dd service name",
		},
		TCPPorts:           []uint16{6400, 5200},
		APMInstrumentation: true,
	}
	return proc
}

func createTestWLMProcessStats(wlmProcs []*wmdef.Process, elevatedPermissions bool) map[int32]*procutil.Stats {
	statsByPid := make(map[int32]*procutil.Stats, len(wlmProcs))
	for _, wlmProc := range wlmProcs {
		statsByPid[wlmProc.Pid] = randomProcessStats(wlmProc.CreationTime.UnixMilli(), elevatedPermissions)
	}
	return statsByPid
}

// randRange returns a random number between min and max inclusive [min, max]
func randRange(min, max int) int {
	return rand.IntN(max+1-min) + min
}

// randomUnprivilegedProcessStats returns process stats with reasonable randomized data
func randomProcessStats(createTime int64, withPriviledgedData bool) *procutil.Stats {
	proc := &procutil.Stats{CreateTime: createTime, // 1 second to 1 day ago
		Status:     []string{"U", "D", "R", "S", "T", "W", "Z"}[rand.IntN(7)], // Valid process statuses
		Nice:       int32(randRange(-20, 19)),                                 // -20 (highest priority) to 19 (lowest)
		NumThreads: int32(randRange(1, 500)),                                  // Most processes use <100 threads, upper bound for heavy apps

		CPUPercent: &procutil.CPUPercentStat{
			UserPct:   rand.Float64() * 100, // Simulate 0-100% user CPU usage
			SystemPct: rand.Float64() * 100, // Simulate 0-100% system CPU usage
		},

		CPUTime: &procutil.CPUTimesStat{
			User:      rand.Float64() * 10000, // Seconds spent in user mode
			System:    rand.Float64() * 5000,  // Seconds spent in kernel mode
			Idle:      rand.Float64() * 20000, // Idle time on thread pools or waiting
			Nice:      rand.Float64() * 1000,  // Niceness-adjusted CPU time
			Iowait:    rand.Float64() * 1000,  // Waiting on IO
			Irq:       rand.Float64() * 500,   // Time servicing IRQs
			Softirq:   rand.Float64() * 500,   // Time servicing soft IRQs
			Steal:     rand.Float64() * 100,   // Time stolen by hypervisor
			Guest:     rand.Float64() * 50,    // Guest VM time (if applicable)
			GuestNice: rand.Float64() * 50,    // Guest time with nice value
			Stolen:    rand.Float64() * 10,    // Time stolen from a virtual CPU
			Timestamp: time.Now().Unix(),      // Capture time
		},

		MemInfo: &procutil.MemoryInfoStat{
			RSS:  uint64(randRange(1<<20, 500<<20)), // 1MB–500MB resident memory
			VMS:  uint64(randRange(10<<20, 5<<30)),  // 10MB–5GB virtual memory
			Swap: uint64(randRange(0, 1<<30)),       // 0–1GB swap
		},

		MemInfoEx: &procutil.MemoryInfoExStat{
			RSS:    uint64(randRange(1<<20, 500<<20)), // should be the same as MemInfo.RSS
			VMS:    uint64(randRange(10<<20, 5<<30)),  // should be the same as MemInfo.VMS
			Shared: uint64(randRange(0, 100<<20)),     // Shared memory (e.g. mmap)
			Text:   uint64(randRange(0, 50<<20)),      // Executable code
			Lib:    uint64(randRange(0, 50<<20)),      // Shared libraries
			Data:   uint64(randRange(1<<20, 2<<30)),   // Heap/stack/data
			Dirty:  uint64(randRange(0, 10<<20)),      // Pages waiting to be written
		},

		CtxSwitches: &procutil.NumCtxSwitchesStat{
			Voluntary:   int64(randRange(0, 1_000_000)), // Caused by blocking or waiting
			Involuntary: int64(randRange(0, 500_000)),   // Caused by CPU scheduler preemption
		},
	}
	if withPriviledgedData {
		proc.IOStat = &procutil.IOCountersStat{
			ReadCount:  int64(randRange(0, 1_000_000)), // Number of read syscalls
			WriteCount: int64(randRange(0, 1_000_000)), // Number of write syscalls
			ReadBytes:  int64(randRange(0, 10<<30)),    // Up to 10GB read
			WriteBytes: int64(randRange(0, 10<<30)),    // Up to 10GB written
		}
		// IORateStat is never populated. TODO: we should probably remove it
		//proc.IORateStat = &procutil.IOCountersRateStat{}
		proc.OpenFdCount = int32(randRange(0, 5000)) // 3 minimum (stdin/out/err) to thousands for busy daemons
	}
	return proc
}
