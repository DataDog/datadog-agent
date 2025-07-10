// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func TestFileMetadataExecs(t *testing.T) {
	SkipIfNotAvailable(t)

	// get arch/abi
	arch := model.UnknownArch
	abi := model.UnknownABI
	switch runtime.GOARCH {
	case "amd64":
		arch = model.X8664
		abi = model.Bit64
	case "386":
		arch = model.X86
		abi = model.Bit32
	case "arm64":
		arch = model.ARM64
		abi = model.Bit64
	case "arm":
		arch = model.ARM
		abi = model.Bit32
	}

	// get os
	fileType := model.Empty
	switch runtime.GOOS {
	case "linux":
		fileType = model.ELFExecutable
	case "darwin":
		fileType = model.MachOExecutable
	case "windows":
		fileType = model.PEExecutable
	default:
		t.Fatal(errors.New("unknown os"))
	}

	ruleDef := &rules.RuleDefinition{
		ID: "test_rule_binary",
		Expression: fmt.Sprintf(`exec.file.name == "ls" && exec.file.metadata.type == %s && exec.file.metadata.is_upx_packed == false && exec.file.metadata.is_garble_obfuscated == false && exec.file.metadata.compression == NONE && exec.file.metadata.is_executable == true`,
			fileType.String()),
	}

	test, err := newTestModule(t, nil, []*rules.RuleDefinition{ruleDef})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("exec-metadata", func(t *testing.T) {
		test.WaitSignal(t, func() error {
			cmd := exec.Command("ls", "-al", "/")
			_ = cmd.Run()
			return nil
		}, test.validateExecEvent(t, noWrapperType, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_binary")
			assert.Equal(t, int(arch), event.Exec.FileMetadata.Architecture)
			assert.Equal(t, int(abi), event.Exec.FileMetadata.ABI)
		}))
	})
}
