// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux && !linux_bpf) || ebpf_bindata
// +build linux,!linux_bpf ebpf_bindata

package ebpf

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/security/config"
)

func getRuntimeCompiledPrograms(config *config.Config, useSyscallWrapper bool) (bytecode.AssetReader, error) {
	return nil, fmt.Errorf("runtime compilation unsupported")
}
