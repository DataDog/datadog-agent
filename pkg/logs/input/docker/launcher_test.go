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
