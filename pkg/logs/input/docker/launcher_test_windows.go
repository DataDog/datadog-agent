// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build docker,windows

package docker

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/stretchr/testify/assert"

	"github.com/docker/docker/api/types"
)

func TestGetFileSourceOnWindows(t *testing.T) {
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
			wantPath:        "c:\\programdata\\docker\\containers\\123456\\123456-json.log",
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
			wantPath:        "c:\\programdata\\docker\\containers\\123456\\123456-json.log",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Launcher{
				serviceNameFunc: tt.sFunc,
			}
			fileSource := l.getFileSource(tt.container, tt.source)
			assert.Equal(t, config.FileType, fileSource.source.Config.Type)
			assert.Equal(t, tt.container.service.Identifier, fileSource.source.Config.Identifier)
			assert.Equal(t, tt.wantServiceName, fileSource.source.Config.Service)
		})
	}
}
