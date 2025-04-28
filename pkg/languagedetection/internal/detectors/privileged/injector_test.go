// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package privileged

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

type proc struct{ pid int32 }

func (p *proc) GetPid() int32 { return p.pid }

func (p *proc) GetCommand() string { panic("unused") }

func (p *proc) GetCmdline() []string { panic("unused") }

func TestLangResult(t *testing.T) {
	pid := os.Getpid()
	p := &proc{pid: int32(pid)}

	detector := NewInjectorDetector()
	_, err := detector.DetectLanguage(p)
	require.Error(t, err)

	var (
		node   = languagemodels.Node
		dotnet = languagemodels.Dotnet
		python = languagemodels.Python
		java   = languagemodels.Java
		ruby   = languagemodels.Ruby
		php    = languagemodels.PHP
	)

	tests := []struct {
		in  string
		out languagemodels.LanguageName
		err bool
	}{
		{
			in:  "nodejs",
			out: node,
		},
		{
			in:  "js",
			out: node,
		},
		{
			in:  "node",
			out: node,
		},
		{
			in:  "php",
			out: php,
		},
		{
			in:  "python",
			out: python,
		},
		{
			in:  "dotnet",
			out: dotnet,
		},
		{
			in:  "ruby",
			out: ruby,
		},
		{
			in:  "jvm",
			out: java,
		},
		{
			in:  "java",
			out: java,
		},
		{
			in:  "banana",
			err: true,
		},
		{
			in:  "asdfasdfkasdfkasdfaskdfad",
			err: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.in, func(t *testing.T) {
			createLangMemfd(t, testCase.in)
			lang, err := detector.DetectLanguage(p)
			if testCase.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, testCase.out, lang.Name)
			}
		})
	}
}

func createLangMemfd(t *testing.T, l string) {
	t.Helper()
	fd, err := memfile(memfdLanguageDetectedFileName, []byte(l))
	require.NoError(t, err)
	t.Cleanup(func() { unix.Close(fd) })
}

// memfile takes a file name used, and the byte slice containing data the file
// should contain.
//
// name does not need to be unique, as it's used only for debugging purposes.
//
// It is up to the caller to close the returned descriptor.
func memfile(name string, b []byte) (int, error) {
	fd, err := unix.MemfdCreate(name, 0)
	if err != nil {
		return 0, fmt.Errorf("MemfdCreate: %v", err)
	}

	err = unix.Ftruncate(fd, int64(len(b)))
	if err != nil {
		return 0, fmt.Errorf("Ftruncate: %v", err)
	}

	data, err := unix.Mmap(fd, 0, len(b), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return 0, fmt.Errorf("Mmap: %v", err)
	}

	copy(data, b)

	err = unix.Munmap(data)
	if err != nil {
		return 0, fmt.Errorf("Munmap: %v", err)
	}

	return fd, nil
}
