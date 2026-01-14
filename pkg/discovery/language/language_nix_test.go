// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package language

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
)

func TestDetectHierarchy(t *testing.T) {
	detector := privileged.NewLanguageDetector()
	data := []struct {
		name           string
		exe            string
		args           []string
		tracerMetadata *tracermetadata.TracerMetadata
		expectedLang   Language
	}{
		{
			name:           "tracer metadata with cpp",
			exe:            "python",
			args:           []string{"python", "myscript.py"},
			tracerMetadata: &tracermetadata.TracerMetadata{TracerLanguage: "cpp"},
			expectedLang:   CPlusPlus,
		},
		{
			name:           "empty tracer metadata",
			exe:            "python",
			args:           []string{"python", "myscript.py"},
			tracerMetadata: &tracermetadata.TracerMetadata{},
			expectedLang:   Python,
		},
		{
			name:           "nil tracer metadata",
			exe:            "python",
			args:           []string{"python", "myscript.py"},
			tracerMetadata: nil,
			expectedLang:   Python,
		},
		{
			name:           "empty exe and args",
			exe:            "",
			args:           []string{},
			tracerMetadata: nil,
			expectedLang:   Go,
		},
	}

	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			lang := Detect(d.exe, d.args, int32(os.Getpid()), detector, d.tracerMetadata)
			require.Equal(t, d.expectedLang, lang)
		})
	}
}

func Test_findInArgs(t *testing.T) {
	data := []struct {
		name string
		exe  string
		args []string
		lang Language
	}{
		{
			name: "empty",
			exe:  "",
			args: nil,
			lang: "",
		},
		{
			name: "simple_java",
			exe:  "",
			args: strings.Split("java -jar MyApp.jar MyApp", " "),
			lang: Java,
		},
		{
			name: "path_java",
			exe:  "",
			args: strings.Split("/usr/bin/java -jar MyApp.jar MyApp", " "),
			lang: Java,
		},
		{
			name: "just_command",
			exe:  "",
			args: strings.Split("./mybinary arg1 arg2 arg3", " "),
			lang: "",
		},
		{
			name: "exe fallback",
			exe:  "/usr/local/bin/python3.10",
			args: strings.Split("gunicorn: worker [foo]", " "),
			lang: Python,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result := FindInArgs(d.exe, d.args)
			if result != d.lang {
				t.Errorf("got %v, want %v", result, d.lang)
			}
		})
	}
}

func TestFindUsingPrivilegedDetector(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep -n 20")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	data := []struct {
		name string
		pid  int32
		res  Language
	}{
		{
			name: "current proc",
			pid:  int32(os.Getpid()),
			res:  Go,
		},
		{
			name: "not go",
			pid:  int32(cmd.Process.Pid),
			res:  "",
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			detector := privileged.NewLanguageDetector()
			lang := FindUsingPrivilegedDetector(detector, d.pid)

			require.Equal(t, d.res, lang)
		})
	}
}
