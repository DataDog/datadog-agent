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
)

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
			name: "just_command",
			args: strings.Split("./mybinary arg1 arg2 arg3", " "),
			lang: "",
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result := FindInArgs(d.args)
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
		envs    map[string]string
		success bool
	}{
		{
			name:    "full",
			args:    []string{fullPath},
			envs:    map[string]string{"PATH": tempDir},
			success: true,
		},
		{
			name:    "full_missing",
			args:    []string{tempDir + "/" + "not_my_file"},
			envs:    map[string]string{"PATH": tempDir},
			success: false,
		},
		{
			name:    "relative_in_path",
			args:    []string{"my_file"},
			envs:    map[string]string{"PATH": tempDir},
			success: true,
		},
		{
			name:    "relative_in_path_missing",
			args:    []string{"not_my_file"},
			envs:    map[string]string{"PATH": tempDir},
			success: false,
		},
		{
			name:    "relative_not_in_path",
			args:    []string{"testdata/dotnet/linuxdotnettest"},
			envs:    map[string]string{"PATH": tempDir},
			success: true,
		},
		{
			name:    "relative_not_in_path_missing",
			args:    []string{"testdata/dotnet/not_my_file"},
			envs:    map[string]string{"PATH": tempDir},
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
