// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
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
			expectedServiceTag: "process_context:svchost",
		},
		{
			name: "nginx",
			cmdline: []string{
				"C:\\nginx-1.23.2\\nginx.exe",
			},
			expectedServiceTag: "process_context:nginx",
		},
		{
			name: "java using the -jar flag",
			cmdline: []string{
				"\"C:\\Program Files\\Java\\jdk-17.0.1\\bin\\java\"", "-Xmx4000m", "-Xms4000m", "-XX:ReservedCodeCacheSize=256m", "-jar", "myService.jar",
			},
			expectedServiceTag: "process_context:myService",
		},
		{
			name: "java with exe extension",
			cmdline: []string{
				"C:\\Program Files\\Java\\jdk-17.0.1\\bin\\java.exe", "com.dog.myService",
			},
			expectedServiceTag: "process_context:myService",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := ddconfig.MockSystemProbe(t)
			mockConfig.Set("service_monitoring_config.process_service_inference.enabled", true)

			proc := procutil.Process{
				Pid:     1,
				Cmdline: tt.cmdline,
			}
			procsByPid := map[int32]*procutil.Process{proc.Pid: &proc}

			se := NewServiceExtractor(mockConfig)
			se.Extract(procsByPid)
			assert.Equal(t, []string{tt.expectedServiceTag}, se.GetServiceContext(proc.Pid))
		})
	}
}

func TestWindowsExtractServiceWithSCMReader(t *testing.T) {
	makeServiceExtractor := func(t *testing.T, sysprobeConfig ddconfig.ConfigReader) (*ServiceExtractor, *mockSCM) {
		se := NewServiceExtractor(sysprobeConfig)
		procsByPid := map[int32]*procutil.Process{1: {
			Pid:     1,
			Cmdline: []string{"C:\\nginx-1.23.2\\nginx.exe"},
		}}
		se.Extract(procsByPid)
		scmReader, mockSCM := newSCMReaderWithMock(t)
		se.scmReader = scmReader
		return se, mockSCM
	}

	t.Run("disabled", func(t *testing.T) {
		cfg := ddconfig.MockSystemProbe(t)
		cfg.Set("service_monitoring_config.process_service_inference.enabled", true)
		cfg.Set("service_monitoring_config.process_service_inference.use_windows_service_name", false)

		se, mockSCM := makeServiceExtractor(t, cfg)

		context := se.GetServiceContext(1)
		assert.Equal(t, []string{"process_context:nginx"}, context)
		mockSCM.AssertNotCalled(t, "GetServiceInfo", mock.Anything)
	})

	t.Run("enabled", func(t *testing.T) {
		cfg := ddconfig.MockSystemProbe(t)
		cfg.Set("service_monitoring_config.process_service_inference.use_windows_service_name", true)
		cfg.Set("service_monitoring_config.process_service_inference.enabled", true)

		se, mockSCM := makeServiceExtractor(t, cfg)
		mockSCM.On("GetServiceInfo", uint64(1)).Return(&winutil.ServiceInfo{
			ServiceName: []string{"test"},
		}, nil)
		context := se.GetServiceContext(1)
		assert.Equal(t, []string{"process_context:test"}, context)
		mockSCM.AssertCalled(t, "GetServiceInfo", uint64(1))
	})

	t.Run("enabled, multiple results", func(t *testing.T) {
		cfg := ddconfig.MockSystemProbe(t)
		cfg.Set("service_monitoring_config.process_service_inference.use_windows_service_name", true)
		cfg.Set("service_monitoring_config.process_service_inference.enabled", true)

		se, mockSCM := makeServiceExtractor(t, cfg)
		mockSCM.On("GetServiceInfo", uint64(1)).Return(&winutil.ServiceInfo{
			ServiceName: []string{"test", "test2"},
		}, nil)
		context := se.GetServiceContext(1)
		assert.Equal(t, []string{"process_context:test", "process_context:test2"}, context)
		mockSCM.AssertCalled(t, "GetServiceInfo", uint64(1))
	})

	t.Run("fallback_to_parsing", func(t *testing.T) {
		cfg := ddconfig.MockSystemProbe(t)
		cfg.Set("service_monitoring_config.process_service_inference.use_windows_service_name", true)
		cfg.Set("service_monitoring_config.process_service_inference.enabled", true)

		se, mockSCM := makeServiceExtractor(t, cfg)
		mockSCM.On("GetServiceInfo", uint64(1)).Return(nil, nil)
		context := se.GetServiceContext(1)
		assert.Equal(t, []string{"process_context:nginx"}, context)
		mockSCM.AssertCalled(t, "GetServiceInfo", uint64(1))
	})
}
