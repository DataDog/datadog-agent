// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package npcollector used to manage network paths
package npcollector

import (
	"iter"

	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// team: network-path

// Component is the component type.
type Component interface {
	ScheduleNetworkPathTests(conns iter.Seq[npmodel.NetworkPathConnection])
	ScheduleNetflowPathTests(conns iter.Seq[npmodel.NetworkPathConnection])
}

// RemoteConfigHandler applies dynamic Network Path Remote Configuration snapshots.
type RemoteConfigHandler interface {
	UpdateRemoteConfig(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus))
}
