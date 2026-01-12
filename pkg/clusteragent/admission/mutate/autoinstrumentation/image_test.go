// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
)

func TestNewImage(t *testing.T) {
	tests := map[string]struct {
		source   string
		wantErr  bool
		expected *autoinstrumentation.Image
	}{
		"java library image parses correctly": {
			source:  "gcr.io/datadoghq/dd-lib-java-init:v1",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "gcr.io/datadoghq",
				Name:     "dd-lib-java-init",
				Tag:      "v1",
			},
		},
		"js library image parses correctly": {
			source:  "gcr.io/datadoghq/dd-lib-js-init:v5",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "gcr.io/datadoghq",
				Name:     "dd-lib-js-init",
				Tag:      "v5",
			},
		},
		"python library image parses correctly": {
			source:  "gcr.io/datadoghq/dd-lib-python-init:v4",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "gcr.io/datadoghq",
				Name:     "dd-lib-python-init",
				Tag:      "v4",
			},
		},
		"dotnet library image parses correctly": {
			source:  "gcr.io/datadoghq/dd-lib-dotnet-init:v3",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "gcr.io/datadoghq",
				Name:     "dd-lib-dotnet-init",
				Tag:      "v3",
			},
		},
		"ruby library image parses correctly": {
			source:  "gcr.io/datadoghq/dd-lib-ruby-init:v2",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "gcr.io/datadoghq",
				Name:     "dd-lib-ruby-init",
				Tag:      "v2",
			},
		},
		"php library image parses correctly": {
			source:  "gcr.io/datadoghq/dd-lib-php-init:v1",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "gcr.io/datadoghq",
				Name:     "dd-lib-php-init",
				Tag:      "v1",
			},
		},
		"gcr us library image parses correctly": {
			source:  "gcr.io/datadoghq/dd-lib-python-init:v4",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "gcr.io/datadoghq",
				Name:     "dd-lib-python-init",
				Tag:      "v4",
			},
		},
		"gcr europe library image parses correctly": {
			source:  "eu.gcr.io/datadoghq/dd-lib-python-init:v4",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "eu.gcr.io/datadoghq",
				Name:     "dd-lib-python-init",
				Tag:      "v4",
			},
		},
		"gcr asia library image parses correctly": {
			source:  "asia.gcr.io/datadoghq/dd-lib-python-init:v4",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "asia.gcr.io/datadoghq",
				Name:     "dd-lib-python-init",
				Tag:      "v4",
			},
		},
		"azure library image parses correctly": {
			source:  "datadoghq.azurecr.io/dd-lib-python-init:v4",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "datadoghq.azurecr.io",
				Name:     "dd-lib-python-init",
				Tag:      "v4",
			},
		},
		"aws library image parses correctly": {
			source:  "public.ecr.aws/datadog/dd-lib-python-init:v4",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "public.ecr.aws/datadog",
				Name:     "dd-lib-python-init",
				Tag:      "v4",
			},
		},
		"docker hub library image parses correctly": {
			source:  "docker.io/datadog/dd-lib-python-init:v4",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "docker.io/datadog",
				Name:     "dd-lib-python-init",
				Tag:      "v4",
			},
		},
		"injector image parses correctly": {
			source:  "gcr.io/datadoghq/apm-inject:0.52.0",
			wantErr: false,
			expected: &autoinstrumentation.Image{
				Registry: "gcr.io/datadoghq",
				Name:     "apm-inject",
				Tag:      "0.52.0",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			img, err := autoinstrumentation.NewImage(test.source)
			if test.wantErr {
				require.Error(t, err, "wanted error parsing image")
				return
			}

			require.NoError(t, err, "no error was expected")
			require.Equal(t, test.expected, img, "the expected image does not match the expected")
			require.Equal(t, test.source, img.String(), "source does not equal string output")
		})
	}
}

func TestToLibrary(t *testing.T) {
	tests := map[string]struct {
		image    *autoinstrumentation.Image
		wantErr  bool
		expected autoinstrumentation.Library
	}{
		"python image converts to python library": {
			image: &autoinstrumentation.Image{
				Registry: "gcr.io/datadoghq",
				Name:     "dd-lib-python-init",
				Tag:      "v4",
			},
			wantErr: false,
			expected: autoinstrumentation.Library{
				Language: autoinstrumentation.Python,
				Version:  "v4",
			},
		},
		"injector image fails to convert to library": {
			image: &autoinstrumentation.Image{
				Registry: "gcr.io/datadoghq",
				Name:     "apm-inject",
				Tag:      "0.52.0",
			},
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lib, err := test.image.ToLibrary()
			if test.wantErr {
				require.Error(t, err, "wanted error converting library")
				return
			}

			require.NoError(t, err, "no error was expected")
			require.Equal(t, test.expected, lib)
		})
	}
}
