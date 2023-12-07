// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

func preRegister(cfg *config.Config) error {
	if !cfg.RequiresEBPF() {
		return nil
	}
	return ebpf.Setup(ebpf.NewConfig())
}

func postRegister(cfg *config.Config) error {
	if !cfg.RequiresEBPF() {
		return nil
	}
	ebpf.FlushBTF()
	return nil
}
