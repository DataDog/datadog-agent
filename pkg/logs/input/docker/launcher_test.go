// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker

package docker

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/stretchr/testify/assert"

	"github.com/docker/docker/api/types"
)

func TestOverrideSourceServiceNameOrder(t *testing.T) {
	tests := []struct {
		name            string
		sFunc           func(string, string) string
		container       *Container
		source          *config.LogSource
		wantServiceName string
	}{
		{
			name:  "log config",
			sFunc: func(n, e string) string { return "stdServiceName" },
			container: &Container{
				container: types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						Name:  "fooName",
						Image: "fooImage",
					},
				},
			},
			source: &config.LogSource{
				Name: "from container",
				Config: &config.LogsConfig{
					Service: "configServiceName",
				},
			},
			wantServiceName: "configServiceName",
		},
		{
			name:  "standard tags",
			sFunc: func(n, e string) string { return "stdServiceName" },
			container: &Container{
				container: types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						Name:  "fooName",
						Image: "fooImage",
					},
				},
			},
			source: &config.LogSource{
				Name:   "from container",
				Config: &config.LogsConfig{},
			},
			wantServiceName: "stdServiceName",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Launcher{
				serviceNameFunc: tt.sFunc,
			}
			if got := l.overrideSource(tt.container, tt.source); !reflect.DeepEqual(got.Config.Service, tt.wantServiceName) {
				t.Errorf("Launcher.overrideSource() = %v, want %v", got.Config.Service, tt.wantServiceName)
			}
		})
	}
}

func TestNewOverridenSourceServiceNameOrder(t *testing.T) {
	tests := []struct {
		name            string
		standardService string
		shortName       string
		status          *config.LogStatus
		wantServiceName string
	}{
		{
			name:            "standard svc name",
			standardService: "stdServiceName",
			shortName:       "fooName",
			status:          config.NewLogStatus(),
			wantServiceName: "stdServiceName",
		},
		{
			name:            "image name",
			standardService: "",
			shortName:       "fooName",
			status:          config.NewLogStatus(),
			wantServiceName: "fooName",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newOverridenSource(tt.standardService, tt.shortName, tt.status); !reflect.DeepEqual(got.Config.Service, tt.wantServiceName) {
				t.Errorf("newOverridenSource() = %v, want %v", got.Config.Service, tt.wantServiceName)
			}
		})
	}
}

func TestGetFileSource(t *testing.T) {
	tests := []struct {
		name            string
		sFunc           func(string, string) string
		container       *Container
		source          *config.LogSource
		wantServiceName string
		wantPath        string
	}{
		{
			name:  "service name from log config",
			sFunc: func(n, e string) string { return "stdServiceName" },
			container: &Container{
				container: types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						Name:  "fooName",
						Image: "fooImage",
					},
				},
				service: &service.Service{Identifier: "123456"},
			},
			source:          config.NewLogSource("from container", &config.LogsConfig{Service: "configServiceName"}),
			wantServiceName: "configServiceName",
			wantPath:        "/var/lib/docker/containers/123456/123456-json.log",
		},
		{
			name:  "service name from standard tags",
			sFunc: func(n, e string) string { return "stdServiceName" },
			container: &Container{
				container: types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						Name:  "fooName",
						Image: "fooImage",
					},
				},
				service: &service.Service{Identifier: "123456"},
			},
			source:          config.NewLogSource("from container", &config.LogsConfig{}),
			wantServiceName: "stdServiceName",
			wantPath:        "/var/lib/docker/containers/123456/123456-json.log",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Launcher{
				serviceNameFunc: tt.sFunc,
			}
			fileSource := l.getFileSource(tt.container, tt.source)
			assert.Equal(t, config.FileType, fileSource.Config.Type)
			assert.Equal(t, tt.container.service.Identifier, fileSource.Config.Identifier)
			assert.Equal(t, tt.wantServiceName, fileSource.Config.Service)
		})
	}
}
