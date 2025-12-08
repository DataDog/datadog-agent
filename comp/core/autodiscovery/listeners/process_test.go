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

	taggerComponent := taggerfxmock.SetupFakeTagger(t)

	tests := []struct {
		name             string
		process          *workloadmeta.Process
		expectedServices map[string]wlmListenerSvc
	}{
		{
			name:    "basic process with service data",
			process: basicProcess,
			expectedServices: map[string]wlmListenerSvc{
				"process://1234": {
					service: &ProcessService{
						process: basicProcess,
						adIdentifiers: []string{
							"redis-server",
							"redis",
						},
						hosts: map[string]string{"host": "127.0.0.1"},
						ports: []ContainerPort{
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
						adIdentifiers: []string{
							"nginx",
						},
						hosts: map[string]string{"host": "127.0.0.1"},
						ports: []ContainerPort{
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
			name:    "process with same comm and generated name only has one identifier",
			process: processWithSameCommAndGeneratedName,
			expectedServices: map[string]wlmListenerSvc{
				"process://1234": {
					service: &ProcessService{
						process: processWithSameCommAndGeneratedName,
						adIdentifiers: []string{
							"postgres",
						},
						hosts: map[string]string{"host": "127.0.0.1"},
						ports: []ContainerPort{
							{Port: 5432},
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
			listener, wlm := newProcessListener(t, taggerComponent)

			if tt.process != nil {
				listener.Store().(workloadmetamock.Mock).Set(tt.process)
			}

			listener.createProcessService(tt.process)

			assertProcessServices(t, wlm, tt.expectedServices)
		})
	}
}

func TestComputeProcessServiceIDs(t *testing.T) {
	tests := []struct {
		name    string
		process *workloadmeta.Process
		want    []string
	}{
		{
			name: "process with different comm and generated name",
			process: &workloadmeta.Process{
				Comm: "redis-server",
				Service: &workloadmeta.Service{
					GeneratedName: "redis",
				},
			},
			want: []string{"redis-server", "redis"},
		},
		{
			name: "process with same comm and generated name",
			process: &workloadmeta.Process{
				Comm: "postgres",
				Service: &workloadmeta.Service{
					GeneratedName: "postgres",
				},
			},
			want: []string{"postgres"},
		},
		{
			name: "process with no generated name",
			process: &workloadmeta.Process{
				Comm:    "mysqld",
				Service: &workloadmeta.Service{},
			},
			want: []string{"mysqld"},
		},
		{
			name: "process with empty comm",
			process: &workloadmeta.Process{
				Comm: "",
				Service: &workloadmeta.Service{
					GeneratedName: "someservice",
				},
			},
			want: []string{"someservice"},
		},
		{
			name: "process with nil service",
			process: &workloadmeta.Process{
				Comm: "bash",
			},
			want: []string{"bash"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeProcessServiceIDs(tt.process)
			assert.Equal(t, tt.want, got)
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
		process:       process,
		adIdentifiers: []string{"redis-server", "redis"},
		hosts:         map[string]string{"host": "127.0.0.1"},
		ports:         []ContainerPort{{Port: 6379}},
		pid:           1234,
		ready:         true,
		tagger:        taggerComponent,
	}

	t.Run("GetServiceID", func(t *testing.T) {
		assert.Equal(t, "process://1234", svc.GetServiceID())
	})

	t.Run("GetADIdentifiers includes cel://process", func(t *testing.T) {
		ids := svc.GetADIdentifiers()
		assert.Contains(t, ids, "redis-server")
		assert.Contains(t, ids, "redis")
		assert.Contains(t, ids, "cel://process")
	})

	t.Run("GetHosts", func(t *testing.T) {
		hosts, err := svc.GetHosts()
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"host": "127.0.0.1"}, hosts)
	})

	t.Run("GetPorts", func(t *testing.T) {
		ports, err := svc.GetPorts()
		assert.NoError(t, err)
		assert.Equal(t, []ContainerPort{{Port: 6379}}, ports)
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
		process:       process,
		tagsHash:      "hash1",
		adIdentifiers: []string{"redis-server"},
		hosts:         map[string]string{"host": "127.0.0.1"},
		ports:         []ContainerPort{{Port: 6379}},
		pid:           1234,
		ready:         true,
	}

	svc2 := &ProcessService{
		process:       process,
		tagsHash:      "hash1",
		adIdentifiers: []string{"redis-server"},
		hosts:         map[string]string{"host": "127.0.0.1"},
		ports:         []ContainerPort{{Port: 6379}},
		pid:           1234,
		ready:         true,
	}

	svc3 := &ProcessService{
		process:       process,
		tagsHash:      "hash2", // different hash
		adIdentifiers: []string{"redis-server"},
		hosts:         map[string]string{"host": "127.0.0.1"},
		ports:         []ContainerPort{{Port: 6379}},
		pid:           1234,
		ready:         true,
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
	wlm := newTestWorkloadmetaListener(t)

	return &ProcessListener{
		workloadmetaListener: wlm,
		tagger:               tagger,
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

