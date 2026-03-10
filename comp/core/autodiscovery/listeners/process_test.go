// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
)

const (
	testPID        = "1234"
	testComm       = "redis-server"
	testPidInt     = int32(1234)
	testPpidInt    = int32(1)
	testParentComm = "init"
)

func TestCreateProcessService(t *testing.T) {
	processEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindProcess,
		ID:   testPID,
	}

	basicProcess := &workloadmeta.Process{
		EntityID: processEntityID,
		Pid:      testPidInt,
		Ppid:     testPpidInt,
		Comm:     testComm,
		Cmdline:  []string{"/usr/bin/redis-server", "--port", "6379"},
		Service: &workloadmeta.Service{
			GeneratedName: "redis",
			TCPPorts:      []uint16{6379},
		},
	}

	processWithMultiplePorts := &workloadmeta.Process{
		EntityID: processEntityID,
		Pid:      testPidInt,
		Ppid:     testPpidInt,
		Comm:     "nginx",
		Cmdline:  []string{"/usr/sbin/nginx"},
		Service: &workloadmeta.Service{
			GeneratedName: "nginx",
			TCPPorts:      []uint16{443, 80},
		},
	}

	processWithNoService := &workloadmeta.Process{
		EntityID: processEntityID,
		Pid:      testPidInt,
		Ppid:     testPpidInt,
		Comm:     "bash",
		Cmdline:  []string{"/bin/bash"},
	}

	containerBoundProcess := &workloadmeta.Process{
		EntityID:    processEntityID,
		Pid:         testPidInt,
		Ppid:        testPpidInt,
		Comm:        testComm,
		Cmdline:     []string{"/usr/bin/redis-server", "--port", "6379"},
		ContainerID: "abc123def456",
		Service: &workloadmeta.Service{
			GeneratedName: "redis",
			TCPPorts:      []uint16{6379},
		},
	}

	processWithSameCommAndGeneratedName := &workloadmeta.Process{
		EntityID: processEntityID,
		Pid:      testPidInt,
		Ppid:     testPpidInt,
		Comm:     "postgres",
		Cmdline:  []string{"/usr/bin/postgres"},
		Service: &workloadmeta.Service{
			GeneratedName: "postgres",
			TCPPorts:      []uint16{5432},
		},
	}

	// Worker process detection test processes
	nginxMain := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "100"},
		Pid:      100,
		Ppid:     1, // Parent is init
		Comm:     "nginx",
		Cmdline:  []string{"/usr/sbin/nginx"},
		Service: &workloadmeta.Service{
			GeneratedName: "nginx",
			TCPPorts:      []uint16{80},
		},
	}

	nginxWorker := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "200"},
		Pid:      200,
		Ppid:     100, // Parent is nginxMain
		Comm:     "nginx",
		Cmdline:  []string{"/usr/sbin/nginx", "-g", "daemon off;"},
		Service: &workloadmeta.Service{
			GeneratedName: "nginx",
			TCPPorts:      []uint16{80},
		},
	}

	pythonParent := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "100"},
		Pid:      100,
		Ppid:     1, // Parent is init
		Comm:     "python",
		Cmdline:  []string{"/usr/bin/python", "app.py"},
		Service: &workloadmeta.Service{
			GeneratedName: "python",
			TCPPorts:      []uint16{8000},
		},
	}

	redisProcess := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "200"},
		Pid:      200,
		Ppid:     100, // Parent is pythonParent
		Comm:     "redis-server",
		Cmdline:  []string{"/usr/bin/redis-server", "--port", "6379"},
		Service: &workloadmeta.Service{
			GeneratedName: "redis",
			TCPPorts:      []uint16{6379},
		},
	}

	taggerComponent := taggerfxmock.SetupFakeTagger(t)

	tests := []struct {
		name             string
		process          *workloadmeta.Process
		setupProcesses   []*workloadmeta.Process
		expectedServices map[string]wlmListenerSvc
	}{
		{
			name:    "basic process with service data",
			process: basicProcess,
			expectedServices: map[string]wlmListenerSvc{
				"process://1234": {
					service: &ProcessService{
						process: basicProcess,
						hosts:   map[string]string{"host": "127.0.0.1"},
						ports: []workloadmeta.ContainerPort{
							{Port: 6379},
						},
						pid:   int(testPidInt),
						ready: true,
					},
				},
			},
		},
		{
			name:    "process with multiple ports sorted in ascending order",
			process: processWithMultiplePorts,
			expectedServices: map[string]wlmListenerSvc{
				"process://1234": {
					service: &ProcessService{
						process: processWithMultiplePorts,
						hosts:   map[string]string{"host": "127.0.0.1"},
						ports: []workloadmeta.ContainerPort{
							{Port: 80},
							{Port: 443},
						},
						pid:   int(testPidInt),
						ready: true,
					},
				},
			},
		},
		{
			name:             "process without service data is skipped",
			process:          processWithNoService,
			expectedServices: map[string]wlmListenerSvc{},
		},
		{
			name:             "container-bound process is skipped",
			process:          containerBoundProcess,
			expectedServices: map[string]wlmListenerSvc{},
		},
		{
			name:    "process with same comm and generated name",
			process: processWithSameCommAndGeneratedName,
			expectedServices: map[string]wlmListenerSvc{
				"process://1234": {
					service: &ProcessService{
						process: processWithSameCommAndGeneratedName,
						hosts:   map[string]string{"host": "127.0.0.1"},
						ports: []workloadmeta.ContainerPort{
							{Port: 5432},
						},
						pid:   int(testPidInt),
						ready: true,
					},
				},
			},
		},
		{
			name:    "nginx worker with nginx parent - no service created",
			process: nginxWorker,
			setupProcesses: []*workloadmeta.Process{
				nginxMain,
			},
			expectedServices: map[string]wlmListenerSvc{},
		},
		{
			name:           "nginx main with init parent - service created",
			process:        nginxMain,
			setupProcesses: []*workloadmeta.Process{},
			expectedServices: map[string]wlmListenerSvc{
				"process://100": {
					service: &ProcessService{
						process: nginxMain,
						hosts:   map[string]string{"host": "127.0.0.1"},
						ports: []workloadmeta.ContainerPort{
							{Port: 80},
						},
						pid:   100,
						ready: true,
					},
				},
			},
		},
		{
			name:    "redis under python parent - service created",
			process: redisProcess,
			setupProcesses: []*workloadmeta.Process{
				pythonParent,
			},
			expectedServices: map[string]wlmListenerSvc{
				"process://200": {
					service: &ProcessService{
						process: redisProcess,
						hosts:   map[string]string{"host": "127.0.0.1"},
						ports: []workloadmeta.ContainerPort{
							{Port: 6379},
						},
						pid:   200,
						ready: true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener, wlm := newProcessListener(t, taggerComponent)

			// Set up parent/ancestor processes in workloadmeta
			for _, proc := range tt.setupProcesses {
				listener.Store().(workloadmetamock.Mock).Set(proc)
			}

			if tt.process != nil {
				listener.Store().(workloadmetamock.Mock).Set(tt.process)
			}

			listener.createProcessService(tt.process)

			assertProcessServices(t, wlm, tt.expectedServices)
		})
	}
}

func TestProcessServiceInterface(t *testing.T) {
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "1234",
		},
		Pid:  1234,
		Comm: "redis-server",
		Service: &workloadmeta.Service{
			GeneratedName: "redis",
			TCPPorts:      []uint16{6379},
		},
	}

	taggerComponent := taggerfxmock.SetupFakeTagger(t)

	svc := &ProcessService{
		process: process,
		hosts:   map[string]string{"host": "127.0.0.1"},
		ports:   []workloadmeta.ContainerPort{{Port: 6379}},
		pid:     1234,
		ready:   true,
		tagger:  taggerComponent,
	}

	t.Run("GetServiceID", func(t *testing.T) {
		assert.Equal(t, "process://1234", svc.GetServiceID())
	})

	t.Run("GetADIdentifiers returns cel://process", func(t *testing.T) {
		ids := svc.GetADIdentifiers()
		assert.Equal(t, []string{"cel://process"}, ids)
	})

	t.Run("GetHosts", func(t *testing.T) {
		hosts, err := svc.GetHosts()
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"host": "127.0.0.1"}, hosts)
	})

	t.Run("GetPorts", func(t *testing.T) {
		ports, err := svc.GetPorts()
		assert.NoError(t, err)
		assert.Equal(t, []workloadmeta.ContainerPort{{Port: 6379}}, ports)
	})

	t.Run("GetPid", func(t *testing.T) {
		pid, err := svc.GetPid()
		assert.NoError(t, err)
		assert.Equal(t, 1234, pid)
	})

	t.Run("GetHostname", func(t *testing.T) {
		hostname, err := svc.GetHostname()
		assert.NoError(t, err)
		assert.Empty(t, hostname)
	})

	t.Run("IsReady", func(t *testing.T) {
		assert.True(t, svc.IsReady())
	})

	t.Run("HasFilter returns false", func(t *testing.T) {
		assert.False(t, svc.HasFilter(""))
	})

	t.Run("GetImageName returns empty", func(t *testing.T) {
		assert.Empty(t, svc.GetImageName())
	})

	t.Run("GetExtraConfig returns error", func(t *testing.T) {
		_, err := svc.GetExtraConfig("test")
		assert.Error(t, err)
	})
}

func TestProcessServiceEqual(t *testing.T) {
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   "1234",
		},
		Pid:  1234,
		Comm: "redis-server",
	}

	svc1 := &ProcessService{
		process:  process,
		tagsHash: "hash1",
		hosts:    map[string]string{"host": "127.0.0.1"},
		ports:    []workloadmeta.ContainerPort{{Port: 6379}},
		pid:      1234,
		ready:    true,
	}

	svc2 := &ProcessService{
		process:  process,
		tagsHash: "hash1",
		hosts:    map[string]string{"host": "127.0.0.1"},
		ports:    []workloadmeta.ContainerPort{{Port: 6379}},
		pid:      1234,
		ready:    true,
	}

	svc3 := &ProcessService{
		process:  process,
		tagsHash: "hash2", // different hash
		hosts:    map[string]string{"host": "127.0.0.1"},
		ports:    []workloadmeta.ContainerPort{{Port: 6379}},
		pid:      1234,
		ready:    true,
	}

	t.Run("equal services", func(t *testing.T) {
		assert.True(t, svc1.Equal(svc2))
	})

	t.Run("different tags hash", func(t *testing.T) {
		assert.False(t, svc1.Equal(svc3))
	})

	t.Run("different type", func(t *testing.T) {
		assert.False(t, svc1.Equal(&WorkloadService{}))
	})
}

func newProcessListener(t *testing.T, tagger tagger.Component) (*ProcessListener, *testWorkloadmetaListener) {
	return newProcessListenerWithFilters(t, tagger, nil)
}

func newProcessListenerWithFilters(t *testing.T, tagger tagger.Component, processFilters workloadfilter.FilterBundle) (*ProcessListener, *testWorkloadmetaListener) {
	wlm := newTestWorkloadmetaListener(t)

	return &ProcessListener{
		workloadmetaListener: wlm,
		tagger:               tagger,
		processFilters:       processFilters,
	}, wlm
}

func assertProcessServices(t *testing.T, wlm *testWorkloadmetaListener, expectedServices map[string]wlmListenerSvc) {
	for svcID, expectedSvc := range expectedServices {
		actualSvc, ok := wlm.services[svcID]
		if !ok {
			t.Errorf("expected to find service %q, but it was not generated", svcID)
			continue
		}

		if diff := cmp.Diff(expectedSvc, actualSvc,
			cmp.AllowUnexported(wlmListenerSvc{}, ProcessService{}),
			cmpopts.IgnoreFields(ProcessService{}, "tagger", "wmeta", "tagsHash")); diff != "" {
			t.Errorf("service %q mismatch (-want +got):\n%s", svcID, diff)
		}

		delete(wlm.services, svcID)
	}

	if len(wlm.services) > 0 {
		for svcID := range wlm.services {
			t.Errorf("got unexpected service: %s", svcID)
		}
	}
}

// mockFilterBundle is a mock implementation of workloadfilter.FilterBundle for testing
type mockFilterBundle struct {
	excludeAll bool
}

func (m *mockFilterBundle) IsExcluded(_ workloadfilter.Filterable) bool {
	return m.excludeAll
}

func (m *mockFilterBundle) GetResult(_ workloadfilter.Filterable) workloadfilter.Result {
	if m.excludeAll {
		return workloadfilter.Excluded
	}
	return workloadfilter.Unknown
}

func (m *mockFilterBundle) GetErrors() []error {
	return nil
}

func TestCreateProcessServiceWithFilters(t *testing.T) {
	processEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindProcess,
		ID:   testPID,
	}

	basicProcess := &workloadmeta.Process{
		EntityID: processEntityID,
		Pid:      testPidInt,
		Ppid:     testPpidInt,
		Comm:     testComm,
		Cmdline:  []string{"/usr/bin/redis-server", "--port", "6379"},
		Service: &workloadmeta.Service{
			GeneratedName: "redis",
			TCPPorts:      []uint16{6379},
		},
	}

	taggerComponent := taggerfxmock.SetupFakeTagger(t)

	tests := []struct {
		name             string
		process          *workloadmeta.Process
		processFilters   workloadfilter.FilterBundle
		expectedServices map[string]wlmListenerSvc
	}{
		{
			name:             "process excluded by filter is skipped",
			process:          basicProcess,
			processFilters:   &mockFilterBundle{excludeAll: true},
			expectedServices: map[string]wlmListenerSvc{
				// No services should be created
			},
		},
		{
			name:           "process not excluded by filter creates service",
			process:        basicProcess,
			processFilters: &mockFilterBundle{excludeAll: false},
			expectedServices: map[string]wlmListenerSvc{
				"process://1234": {
					service: &ProcessService{
						process: basicProcess,
						hosts:   map[string]string{"host": "127.0.0.1"},
						ports: []workloadmeta.ContainerPort{
							{Port: 6379},
						},
						pid:   int(testPidInt),
						ready: true,
					},
				},
			},
		},
		{
			name:           "nil filter bundle creates service",
			process:        basicProcess,
			processFilters: nil,
			expectedServices: map[string]wlmListenerSvc{
				"process://1234": {
					service: &ProcessService{
						process: basicProcess,
						hosts:   map[string]string{"host": "127.0.0.1"},
						ports: []workloadmeta.ContainerPort{
							{Port: 6379},
						},
						pid:   int(testPidInt),
						ready: true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener, wlm := newProcessListenerWithFilters(t, taggerComponent, tt.processFilters)

			if tt.process != nil {
				listener.Store().(workloadmetamock.Mock).Set(tt.process)
			}

			listener.createProcessService(tt.process)

			assertProcessServices(t, wlm, tt.expectedServices)
		})
	}
}

func TestIsMainProcessForService(t *testing.T) {
	taggerComponent := taggerfxmock.SetupFakeTagger(t)
	listener, _ := newProcessListener(t, taggerComponent)
	store := listener.Store().(workloadmetamock.Mock)

	tests := []struct {
		name           string
		process        *workloadmeta.Process
		setupProcesses []*workloadmeta.Process
		expected       bool
	}{
		{
			name: "ppid 0 - no parent",
			process: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "100"},
				Pid:      100,
				Ppid:     0,
				Comm:     "nginx",
				Service:  &workloadmeta.Service{GeneratedName: "nginx", TCPPorts: []uint16{80}},
			},
			expected: true,
		},
		{
			name: "ppid 1 - init parent",
			process: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "100"},
				Pid:      100,
				Ppid:     1,
				Comm:     "redis-server",
				Service:  &workloadmeta.Service{GeneratedName: "redis", TCPPorts: []uint16{6379}},
			},
			expected: true,
		},
		{
			name: "non-existent parent",
			process: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "200"},
				Pid:      200,
				Ppid:     999,
				Comm:     "postgres",
				Service:  &workloadmeta.Service{GeneratedName: "postgres", TCPPorts: []uint16{5432}},
			},
			expected: true,
		},
		{
			name: "parent without service",
			process: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "300"},
				Pid:      300,
				Ppid:     200,
				Comm:     "nginx",
				Service:  &workloadmeta.Service{GeneratedName: "nginx", TCPPorts: []uint16{80}},
			},
			setupProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "200"},
					Pid:      200,
					Ppid:     1,
					Comm:     "bash",
					Service:  nil,
				},
			},
			expected: true,
		},
		{
			name: "different GeneratedName parent - main process",
			process: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "400"},
				Pid:      400,
				Ppid:     300,
				Comm:     "redis-server",
				Service:  &workloadmeta.Service{GeneratedName: "redis", TCPPorts: []uint16{6379}},
			},
			setupProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "300"},
					Pid:      300,
					Ppid:     1,
					Comm:     "python",
					Service:  &workloadmeta.Service{GeneratedName: "python", TCPPorts: []uint16{8000}},
				},
			},
			expected: true,
		},
		{
			name: "same comm different GeneratedName - different services",
			process: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "800"},
				Pid:      800,
				Ppid:     700,
				Comm:     "python",
				Service:  &workloadmeta.Service{GeneratedName: "flask", TCPPorts: []uint16{5000}},
			},
			setupProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "700"},
					Pid:      700,
					Ppid:     1,
					Comm:     "python",
					Service:  &workloadmeta.Service{GeneratedName: "supervisord", TCPPorts: []uint16{9001}},
				},
			},
			expected: true,
		},
		{
			name: "same comm same GeneratedName - worker",
			process: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "900"},
				Pid:      900,
				Ppid:     800,
				Comm:     "python",
				Service:  &workloadmeta.Service{GeneratedName: "myapp", TCPPorts: []uint16{8000}},
			},
			setupProcesses: []*workloadmeta.Process{
				{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "800"},
					Pid:      800,
					Ppid:     1,
					Comm:     "python",
					Service:  &workloadmeta.Service{GeneratedName: "myapp", TCPPorts: []uint16{8000}},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up parent processes if any
			for _, proc := range tt.setupProcesses {
				store.Set(proc)
			}

			// Call the function being tested
			result := isMainProcessForService(tt.process, store)

			// Assert the result
			assert.Equal(t, tt.expected, result, "isMainProcessForService returned unexpected result")

			// Clean up - unset all processes for next test
			for _, proc := range tt.setupProcesses {
				store.Unset(proc)
			}
		})
	}
}
