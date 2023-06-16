// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (linux && !linux_bpf) || ebpf_bindata

package ebpf

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-go/v5/statsd"
)

func getRuntimeCompiledPrograms(config *config.Config, useSyscallWrapper, useRingBuffer bool, client statsd.ClientInterface) (bytecode.AssetReader, error) {
	return nil, fmt.Errorf("runtime compilation unsupported")
}
