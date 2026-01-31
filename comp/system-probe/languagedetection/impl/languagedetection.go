// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package languagedetectionimpl implements the languagedetection component interface
package languagedetectionimpl

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	languagedetection "github.com/DataDog/datadog-agent/comp/system-probe/languagedetection/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
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
	mc := &module.Component{
		Factory: modules.LanguageDetectionModule,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.LanguageDetectionModule.Fn(nil, sysmodule.FactoryDependencies{})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
