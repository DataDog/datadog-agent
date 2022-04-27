// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && !ebpf_bindata
// +build linux_bpf,!ebpf_bindata

package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/security/config"
)

func getRuntimeCompiledPrograms(config *config.Config, useSyscallWrapper bool) (bytecode.AssetReader, error) {
	cflags := runtime.GetSecurityAssetCFlags(useSyscallWrapper)
	compiledOutput, err := runtime.RuntimeSecurity.GetCompiledOutput(cflags, config.RuntimeCompiledAssetDir)
	if err != nil {
		return nil, err
	}
	return compiledOutput, nil
}
