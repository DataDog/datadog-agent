// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module is the scaffolding for a system-probe module and the loader used upon start
package module

import (
	"errors"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
)

// ErrNotEnabled is a special error type that should be returned by a Factory
// when the associated Module is not enabled.
var ErrNotEnabled = errors.New("module is not enabled")

// Module defines the common API implemented by every System Probe Module
type Module interface {
	GetStats() map[string]interface{}
	Register(*Router) error
	Close()
}

// FactoryDependencies defines the fx dependencies for a module factory
type FactoryDependencies struct {
	fx.In

	WMeta       workloadmeta.Component
	Tagger      tagger.Component
	Telemetry   telemetry.Component
	Compression logscompression.Component
	Statsd      ddgostatsd.ClientInterface
	Hostname    hostname.Component
	Ipc         ipc.Component
}
