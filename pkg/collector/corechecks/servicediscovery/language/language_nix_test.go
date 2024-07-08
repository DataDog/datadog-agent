// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package language

import (
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	f := New(zap.NewNop())
	// make sure all alternatives are registered
	if f.Logger == nil {
		t.Error("Logger is nil")
	}
	count := 0
	for _, v := range f.Matchers {
		switch v.(type) {
		case PythonScript:
			count |= 1
		case RubyScript:
			count |= 2
		case DotNetBinary:
			count |= 4
		}
	}
	if count != 7 {
		t.Error("Missing a matcher")
	}
}

func Test_findInArgs(t *testing.T) {
	data := []struct {
		name string
		args []string
		lang Language
	}{
		{
			name: "empty",
			args: nil,
			lang: "",
		},
		{
			name: "simple_java",
			args: strings.Split("java -jar MyApp.jar MyApp", " "),
			lang: Java,
		},
		{
			name: "path_java",
			args: strings.Split("/usr/bin/java -jar MyApp.jar MyApp", " "),
			lang: Java,
		},
		{
			name: "extra_commands_path_java",
			args: strings.Split("time /usr/bin/java -jar MyApp.jar MyApp", " "),
			lang: Java,
		},
		{
			name: "just_command",
			args: strings.Split("./mybinary arg1 arg2 arg3", " "),
			lang: "",
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result := findInArgs(d.args)
			if result != d.lang {
				t.Errorf("got %v, want %v", result, d.lang)
			}
		})
	}
}

func TestFinder_findLang(t *testing.T) {
	f := New(zap.NewNop())
	data := []struct {
		name string
		pi   ProcessInfo
		lang Language
	}{
		{
			name: "dotnet binary",
			pi: ProcessInfo{
				Args: strings.Split("testdata/dotnet/linuxdotnettest a b c", " "),
				Envs: []string{"PATH=/usr/bin"},
			},
			lang: DotNet,
		},
		{
			name: "dotnet",
			pi: ProcessInfo{
				Args: strings.Split("dotnet run mydll.dll a b c", " "),
				Envs: []string{"PATH=/usr/bin"},
			},
			lang: DotNet,
		},
		{
			name: "native",
			pi: ProcessInfo{
				Args: strings.Split("./myproc a b c", " "),
				Envs: []string{"PATH=/usr/bin"},
			},
			lang: "",
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result := f.findLang(d.pi)
			if result != d.lang {
				t.Errorf("got %v, want %v", result, d.lang)
			}
		})
	}
}
