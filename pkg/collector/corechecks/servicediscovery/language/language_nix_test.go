// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package language

import (
	"os"
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

func TestProcessInfoFileReader(t *testing.T) {
	// create a temp file
	tempDir := t.TempDir()
	fullPath := tempDir + "/" + "my_file"
	err := os.WriteFile(fullPath, []byte("hello"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	data := []struct {
		name    string
		args    []string
		envs    []string
		success bool
	}{
		{
			name:    "full",
			args:    []string{fullPath},
			envs:    []string{"PATH=" + tempDir},
			success: true,
		},
		{
			name:    "full_missing",
			args:    []string{tempDir + "/" + "not_my_file"},
			envs:    []string{"PATH=" + tempDir},
			success: false,
		},
		{
			name:    "relative_in_path",
			args:    []string{"my_file"},
			envs:    []string{"PATH=" + tempDir},
			success: true,
		},
		{
			name:    "relative_in_path_missing",
			args:    []string{"not_my_file"},
			envs:    []string{"PATH=" + tempDir},
			success: false,
		},
		{
			name:    "relative_not_in_path",
			args:    []string{"testdata/dotnet/linuxdotnettest"},
			envs:    []string{"PATH=" + tempDir},
			success: true,
		},
		{
			name:    "relative_not_in_path_missing",
			args:    []string{"testdata/dotnet/not_my_file"},
			envs:    []string{"PATH=" + tempDir},
			success: false,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			pi := ProcessInfo{
				Args: d.args,
				Envs: d.envs,
			}
			rc, ok := pi.FileReader()
			if ok != d.success {
				t.Errorf("got %v, want %v", ok, d.success)
			}
			if rc != nil {
				rc.Close()
			}
		})
	}
}

func TestFinderDetect(t *testing.T) {
	f := New(zap.NewNop())
	data := []struct {
		name string
		args []string
		envs []string
		lang Language
		ok   bool
	}{
		{
			name: "dotnet binary",
			args: strings.Split("testdata/dotnet/linuxdotnettest a b c", " "),
			envs: []string{"PATH=/usr/bin"},
			lang: DotNet,
			ok:   true,
		},
		{
			name: "dotnet",
			args: strings.Split("dotnet run mydll.dll a b c", " "),
			envs: []string{"PATH=/usr/bin"},
			lang: DotNet,
			ok:   true,
		},
		{
			name: "native",
			args: strings.Split("./myproc a b c", " "),
			envs: []string{"PATH=/usr/bin"},
			lang: Unknown,
			ok:   false,
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result, ok := f.Detect(d.args, d.envs)
			if ok != d.ok {
				t.Errorf("got %v, want %v", ok, d.ok)
			}
			if result != d.lang {
				t.Errorf("got %v, want %v", result, d.lang)
			}
		})
	}
}
