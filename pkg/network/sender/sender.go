// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	"github.com/DataDog/datadog-agent/pkg/network"
)

// Sender sends data directly to the backend for CNM/USM data
type Sender interface {
	Stop()
}

// Dependencies are all the component dependencies of the direct sender
type Dependencies struct {
	Config         config.Component
	Logger         log.Component
	Sysprobeconfig sysprobeconfig.Component
	Tagger         tagger.Component
	Wmeta          workloadmeta.Component
	Hostname       hostname.Component
	Forwarder      connectionsforwarder.Component
	NPCollector    npcollector.Component
}

// ConnectionsSource is an interface for an object that can provide network connections data
type ConnectionsSource interface {
	RegisterClient(clientID string) error
	GetActiveConnections(clientID string) (*network.Connections, func(), error)
}
