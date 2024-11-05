// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux_bpf

// Package module provides the dynamic instrumentaiton module. This is a stub meaning
// this empty file is used if the target platform does not support features required
// by dynamic instrumentation.
package module

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
)

//nolint:revive // TODO(DEBUG) Fix revive linter
type Config struct{}

//nolint:revive // TODO(DEBUG) Fix revive linter
func NewConfig(_ *sysconfigtypes.Config) (*Config, error) {
	return &Config{}, nil
}

//nolint:revive // TODO(DEBUG) Fix revive linter
type Module struct {
}

//nolint:revive // TODO(DEBUG) Fix revive linter
func NewModule(config *Config) (*Module, error) {
	return nil, nil
}

//nolint:revive // TODO(DEBUG) Fix revive linter
func (m *Module) Close() {}

//nolint:revive // TODO(DEBUG) Fix revive linter
func (m *Module) GetStats() map[string]interface{} {
	return nil
}

//nolint:revive // TODO(DEBUG) Fix revive linter
func (m *Module) Register(_ *module.Router) error {
	return nil
}
