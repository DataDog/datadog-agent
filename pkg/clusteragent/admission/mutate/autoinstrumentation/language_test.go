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

func TestSupportedLanguages(t *testing.T) {
	supportedLanguages := map[autoinstrumentation.Language]bool{}
	for _, lang := range autoinstrumentation.SupportedLanguages {
		_, ok := autoinstrumentation.DefaultVersions[lang]
		require.True(t, ok, "all supported languages need to have a default version defined")

		_, ok = autoinstrumentation.SupportedLanguagesMap[lang]
		require.True(t, ok, "all supported languages need to be included in the map")

		supportedLanguages[lang] = true
	}

	for lang, _ := range autoinstrumentation.SupportedLanguagesMap {
		_, ok := supportedLanguages[lang]
		require.True(t, ok, "all supported languages in the map need to be included in the slice")
	}
}

func TestDefaultVersions(t *testing.T) {
	for lang, _ := range autoinstrumentation.DefaultVersions {
		_, ok := autoinstrumentation.SupportedLanguagesMap[lang]
		require.True(t, ok, "all default versions need to have a supported language defined")
	}
}

func TestExtractLibraryLanguage(t *testing.T) {
	tests := map[string]struct {
		lib      string
		wantErr  bool
		expected autoinstrumentation.Language
	}{
		"java library input extracts language": {
			lib:      "dd-lib-java-init",
			wantErr:  false,
			expected: autoinstrumentation.Java,
		},
		"js library input extracts language": {
			lib:      "dd-lib-js-init",
			wantErr:  false,
			expected: autoinstrumentation.Javascript,
		},
		"python library input extracts language": {
			lib:      "dd-lib-python-init",
			wantErr:  false,
			expected: autoinstrumentation.Python,
		},
		"dotnet library input extracts language": {
			lib:      "dd-lib-dotnet-init",
			wantErr:  false,
			expected: autoinstrumentation.Dotnet,
		},
		"ruby library input extracts language": {
			lib:      "dd-lib-ruby-init",
			wantErr:  false,
			expected: autoinstrumentation.Ruby,
		},
		"php library input extracts language": {
			lib:      "dd-lib-php-init",
			wantErr:  false,
			expected: autoinstrumentation.PHP,
		},
		"unsupported language causes error": {
			lib:     "dd-lib-go-init",
			wantErr: true,
		},
		"invalid input causes error": {
			lib:     "foo",
			wantErr: true,
		},
		"apm inject input causes error": {
			lib:     "apm-inject",
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lang, err := autoinstrumentation.ExtractLibraryLanguage(test.lib)
			if test.wantErr {
				require.Error(t, err, "error was expected")
				return
			}
			require.NoError(t, err, "no error was expected")
			require.Equal(t, test.expected, lang)
		})
	}
}

func TestNewLanguage(t *testing.T) {
	tests := map[string]struct {
		lang     string
		wantErr  bool
		expected autoinstrumentation.Language
	}{
		"java language": {
			lang:     "java",
			wantErr:  false,
			expected: autoinstrumentation.Java,
		},
		"js language": {
			lang:     "js",
			wantErr:  false,
			expected: autoinstrumentation.Javascript,
		},
		"python language": {
			lang:     "python",
			wantErr:  false,
			expected: autoinstrumentation.Python,
		},
		"dotnet language": {
			lang:     "dotnet",
			wantErr:  false,
			expected: autoinstrumentation.Dotnet,
		},
		"ruby language": {
			lang:     "ruby",
			wantErr:  false,
			expected: autoinstrumentation.Ruby,
		},
		"php language": {
			lang:     "php",
			wantErr:  false,
			expected: autoinstrumentation.PHP,
		},
		"unsupported language causes error": {
			lang:    "go",
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lang, err := autoinstrumentation.NewLanguage(test.lang)
			if test.wantErr {
				require.Error(t, err, "error was expected")
				return
			}
			require.NoError(t, err, "no error was expected")
			require.Equal(t, test.expected, lang)
		})
	}
}
