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

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
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

	SysprobeConfig       sysprobeconfig.Component
	CoreConfig           config.Component
	Log                  log.Component
	WMeta                workloadmeta.Component
	FilterStore          workloadfilter.Component
	Tagger               tagger.Component
	Telemetry            telemetry.Component
	Compression          logscompression.Component
	Secrets              secrets.Component
	Statsd               ddgostatsd.ClientInterface
	Hostname             hostname.Component
	Ipc                  ipc.Component
	Traceroute           traceroute.Component
	ConnectionsForwarder connectionsforwarder.Component
	NPCollector          npcollector.Component
}
