// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package docker

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/docker/docker/api/types"
)

func init() {
	workloadmeta.CreateGlobalStore(nil)
}

func TestOverrideSourceServiceNameOrder(t *testing.T) {
	tests := []struct {
		name            string
		sFunc           func(string, string) string
		container       *Container
		source          *sources.LogSource
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
			source: &sources.LogSource{
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
			source: &sources.LogSource{
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
		status          *status.LogStatus
		wantServiceName string
	}{
		{
			name:            "standard svc name",
			standardService: "stdServiceName",
			shortName:       "fooName",
			status:          status.NewLogStatus(),
			wantServiceName: "stdServiceName",
		},
		{
			name:            "image name",
			standardService: "",
			shortName:       "fooName",
			status:          status.NewLogStatus(),
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

	testRules := []*config.ProcessingRule{
		{Name: "foo", Type: config.IncludeAtMatch, Pattern: "[[:alnum:]]{5}"},
		{Name: "bar", Type: config.ExcludeAtMatch, Pattern: "^plop"},
		{Name: "baz", Type: config.MultiLine, Pattern: "[0-9]"},
	}
	tests := []struct {
		name            string
		sFunc           func(string, string) string
		container       *Container
		source          *sources.LogSource
		wantServiceName string
		wantSourceName  string
		wantPath        string
		wantTags        []string
		wantRules       []*config.ProcessingRule
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
			source:          sources.NewLogSource("from container", &config.LogsConfig{Service: "configServiceName", Source: "configSourceName", Tags: []string{"foo:bar", "foo:baz"}}),
			wantServiceName: "configServiceName",
			wantSourceName:  "configSourceName",
			wantPath:        "/var/lib/docker/containers/123456/123456-json.log",
			wantTags:        []string{"foo:bar", "foo:baz"},
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
			source:          sources.NewLogSource("from container", &config.LogsConfig{ProcessingRules: testRules, Source: "stdSourceName"}),
			wantServiceName: "stdServiceName",
			wantSourceName:  "stdSourceName",
			wantPath:        "/var/lib/docker/containers/123456/123456-json.log",
			wantRules:       testRules,
			wantTags:        nil,
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
			assert.Equal(t, tt.wantSourceName, fileSource.source.Config.Source)
			assert.Equal(t, tt.wantTags, fileSource.source.Config.Tags)
			assert.Equal(t, tt.wantRules, fileSource.source.Config.ProcessingRules)
		})
	}
}
