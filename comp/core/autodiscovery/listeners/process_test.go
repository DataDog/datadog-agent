// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"testing"

	fxtagger "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestProcessListener(t *testing.T) {
	tests := []struct {
		name          string
		process       *workloadmeta.Process
		expectedSvc   *service
		expectedSvcID string
		expectedError bool
	}{
		{
			name: "process with service info",
			process: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{
					ID:   "1234",
					Kind: workloadmeta.KindProcess,
				},
				Pid: 1234,
				Service: &workloadmeta.Service{
					GeneratedName:            "test-service",
					ContainerServiceName:     "container-service",
					DDService:                "dd-service",
					AdditionalGeneratedNames: []string{"alt-name-1", "alt-name-2"},
					Ports:                    []uint16{8080, 8081},
					ContainerTags:            []string{"env:prod", "service:test"},
				},
			},
			expectedSvc: &service{
				entity: &workloadmeta.Process{
					EntityID: workloadmeta.EntityID{
						ID:   "1234",
						Kind: workloadmeta.KindProcess,
					},
					Pid: 1234,
					Service: &workloadmeta.Service{
						GeneratedName:            "test-service",
						ContainerServiceName:     "container-service",
						DDService:                "dd-service",
						AdditionalGeneratedNames: []string{"alt-name-1", "alt-name-2"},
						Ports:                    []uint16{8080, 8081},
						ContainerTags:            []string{"env:prod", "service:test"},
					},
				},
				adIdentifiers: []string{
					"test-service",
					"container-service",
					"dd-service",
					"alt-name-1",
					"alt-name-2",
				},
				ports: []ContainerPort{
					{Port: 8080},
					{Port: 8081},
				},
				pid:      1234,
				hostname: "test-service",
			},
			expectedSvcID: "process://1234",
			expectedError: false,
		},
		{
			name: "process without service info",
			process: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{
					ID:   "1234",
					Kind: workloadmeta.KindProcess,
				},
				Pid: 1234,
			},
			expectedSvc:   nil,
			expectedSvcID: "",
			expectedError: false,
		},
		{
			name: "process with empty service names",
			process: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{
					ID:   "1234",
					Kind: workloadmeta.KindProcess,
				},
				Pid:     1234,
				Service: &workloadmeta.Service{},
			},
			expectedSvc:   nil,
			expectedSvcID: "",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new listener
			tagger := fxtagger.SetupFakeTagger(t)
			l := &ProcessListener{
				tagger: tagger,
			}
			wlm := newTestWorkloadmetaListener(t)
			l.workloadmetaListener = wlm

			// Process the entity
			l.createProcessService(tt.process)

			if tt.expectedSvc == nil {
				// Verify no service was created
				assert.Empty(t, wlm.services)
				return
			}

			// Verify service was created correctly
			assert.Len(t, wlm.services, 1)

			svc, ok := wlm.services[tt.expectedSvcID]
			assert.True(t, ok)
			assert.NotNil(t, svc)

			// Compare service fields
			assert.Equal(t, tt.expectedSvc.adIdentifiers, svc.service.(*service).adIdentifiers)
			assert.Equal(t, tt.expectedSvc.ports, svc.service.(*service).ports)
			assert.Equal(t, tt.expectedSvc.pid, svc.service.(*service).pid)
			assert.Equal(t, tt.expectedSvc.hostname, svc.service.(*service).hostname)
		})
	}
}
