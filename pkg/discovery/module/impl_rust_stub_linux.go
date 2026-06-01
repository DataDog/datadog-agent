// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && !linux_bpf

package module

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

// InitDiscoveryLogger is a no-op in builds without the Rust library.
func InitDiscoveryLogger() {}

// rustGetServices is a stub for builds without the Rust library.
func rustGetServices(_ core.Params) (*model.ServicesResponse, error) {
	return nil, errors.New("rust library not available in this build (requires linux_bpf)")
}
