// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !linux

package module

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/config"
)

// NewModule instantiates a runtime security system-probe module
func NewModule(cfg *config.Config) (api.Module, error) {
	return nil, ebpf.ErrNotImplemented
}
