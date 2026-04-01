// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Stub getServices for Linux builds without linux_bpf or cgo, where the Rust
// implementation is not compiled in.

//go:build linux && (!linux_bpf || !cgo)

package module

import (
	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
)

func (s *discovery) getServices(_ core.Params) (*model.ServicesResponse, error) {
	return &model.ServicesResponse{Services: make([]model.Service, 0)}, nil
}
