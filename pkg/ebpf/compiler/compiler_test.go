// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
)

func TestCompilerMatch(t *testing.T) {
	cPath := "../network/ebpf/c/prebuilt/offset-guess.c"
	if _, err := os.Stat(cPath); err != nil {
		if os.IsNotExist(err) {
			t.Skipf("compiler test must be run in source tree")
		} else {
			t.Fatalf("error checking for offset-guess.c: %s", err)
		}
		return
	}
	cfg := ebpf.NewConfig()

	cflags := []string{
		"-I./c",
		"-I../network/ebpf/c",
		"-includeasm_goto_workaround.h",
	}
	tmpObjFile, err := os.CreateTemp("", "offset-guess-static-*.o")
	require.NoError(t, err)
	defer os.Remove(tmpObjFile.Name())

	onDiskObjFilename := tmpObjFile.Name()
	err = CompileToObjectFile(cPath, onDiskObjFilename, cflags, nil)
	require.NoError(t, err)

	bs, err := os.ReadFile(onDiskObjFilename)
	require.NoError(t, err)

	bundleFilename := "offset-guess.o"
	actualReader, err := bytecode.GetReader(cfg.BPFDir, bundleFilename)
	require.NoError(t, err)
	defer actualReader.Close()

	actual, err := io.ReadAll(actualReader)
	require.NoError(t, err)

	assert.Equal(t, bs, actual, fmt.Sprintf("prebuilt file %s and statically-linked clang compiled content %s are different", bundleFilename, onDiskObjFilename))
}
