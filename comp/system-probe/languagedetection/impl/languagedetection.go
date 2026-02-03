// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package languagedetectionimpl implements the languagedetection component interface
package languagedetectionimpl

import (
	languagedetection "github.com/DataDog/datadog-agent/comp/system-probe/languagedetection/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/privileged"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Requires defines the dependencies for the languagedetection component
type Requires struct {
}

// Provides defines the output of the languagedetection component
type Provides struct {
	Comp   languagedetection.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new languagedetection component
func NewComponent(_ Requires) (Provides, error) {
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			return &languageDetectionModule{
				languageDetector: privileged.NewLanguageDetector(),
			}, nil
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}

type moduleFactory struct {
	createFn func() (types.SystemProbeModule, error)
}

func (m *moduleFactory) Name() sysconfigtypes.ModuleName {
	return config.LanguageDetectionModule
}

func (m *moduleFactory) ConfigNamespaces() []string {
	return []string{"language_detection"}
}

func (m *moduleFactory) Create() (types.SystemProbeModule, error) {
	return m.createFn()
}

func (m *moduleFactory) NeedsEBPF() bool {
	return false
}

func (m *moduleFactory) OptionalEBPF() bool {
	return false
}
