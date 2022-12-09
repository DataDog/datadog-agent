// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestWindowsExtractServiceMetadata(t *testing.T) {
	tests := []struct {
		name               string
		cmdline            []string
		expectedServiceTag string
	}{
		{
			name: "CDPSvc",
			cmdline: []string{
				"C:\\Windows\\system32\\svchost.exe", "-k", "LocalService", "-p", "-s", "CDPSvc",
			},
			expectedServiceTag: "service:svchost",
		},
		{
			name: "nginx",
			cmdline: []string{
				"C:\\nginx-1.23.2\\nginx.exe",
			},
			expectedServiceTag: "service:nginx",
		},
		{
			name: "java using the -jar flag",
			cmdline: []string{
				"\"C:\\Program Files\\Java\\j2re1.4.2_04\\bin\\javaw.exe\"", "-Xmx4000m", "-Xms4000m", "-XX:ReservedCodeCacheSize=256m", "-jar", "myService.jar",
			},
			expectedServiceTag: "service:myservice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := ddconfig.Mock(t)
			mockConfig.Set("service_monitoring_config.process_service_inference.enabled", true)

			proc := procutil.Process{
				Pid:     1,
				Cmdline: tt.cmdline,
			}
			procsByPid := map[int32]*procutil.Process{proc.Pid: &proc}

			se := NewServiceExtractor()
			se.Extract(procsByPid)
			assert.Equal(t, tt.expectedServiceTag, se.GetServiceTag(proc.Pid))
		})
	}
}
