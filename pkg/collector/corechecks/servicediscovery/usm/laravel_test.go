// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
)

func TestGetLaravelAppNameFromEnv(t *testing.T) {
	tests := []struct {
		name       string
		expected   string
		filesystem fs.SubFS
	}{
		{
			name:     "APP_NAME in .env",
			expected: "my-first-name",
			filesystem: fstest.MapFS{
				".env": {Data: []byte("APP_NAME=my-first-name")},
			},
		},
		{
			name:     "DD_SERVICE is prioritized over APP_NAME in .env",
			expected: "backoffice",
			filesystem: fstest.MapFS{
				".env": {Data: []byte("APP_NAME=my-first-name\nDD_SERVICE=backoffice")},
			},
		},
		{
			name:     "OTEL_SERVICE_NAME is prioritized over APP_NAME in .env",
			expected: "backoffice",
			filesystem: fstest.MapFS{
				".env": {Data: []byte("APP_NAME=my-first-name\nOTEL_SERVICE_NAME=backoffice")},
			},
		},
		{
			name:     "DD_SERVICE is prioritized over OTEL_SERVICE_NAME in .env",
			expected: "my-first-name",
			filesystem: fstest.MapFS{
				".env": {Data: []byte("OTEL_SERVICE_NAME=backoffice\nDD_SERVICE=my-first-name")},
			},
		},
		{
			name:     "APP_NAME in config/app.php",
			expected: "my-first-name",
			filesystem: fstest.MapFS{
				"config/": {Mode: fs.ModeDir},
				"config/app.php": {Data: []byte(`<?php
		return [
			'name' =>env('APP_NAME','my-first-name'),
		];`)},
			},
		},
		{
			name:     "APP_NAME in config/app.php non-traditional format",
			expected: "my-first-name",
			filesystem: fstest.MapFS{
				"config/": {Mode: fs.ModeDir},
				"config/app.php": {Data: []byte(`<?php
		return [
			'name'=>"my-first-name",
		];`)},
			},
		},
		{
			name:     "Defaults to laravel",
			expected: "Laravel",
			filesystem: fstest.MapFS{
				".env":    {Data: []byte("FOO=bar")},
				"config/": {Mode: fs.ModeDir},
				"config/app.php": {Data: []byte(`<?php
		return [
			'name' => env('APP_NAME', 'Laravel'),
		];`)},
			},
		},
		{
			name:     ".env is prioritized over config/app.php",
			expected: "my-first-name",
			filesystem: fstest.MapFS{
				".env":    {Data: []byte("APP_NAME=my-first-name")},
				"config/": {Mode: fs.ModeDir},
				"config/app.php": {Data: []byte(`<?php
		return [
			'name' => env('APP_NAME','my-second-name'),
		];`)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := newLaravelParser(NewDetectionContext(nil, envs.NewVariables(nil), tt.filesystem)).GetLaravelAppName("artisan")
			require.Equal(t, tt.expected, name)
		})
	}
}
