// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package windowseventlogimpl provides the Windows Event Log check component
package windowseventlogimpl

import (
	"context"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	windowseventlog "github.com/DataDog/datadog-agent/comp/checks/windowseventlog/def"
	check "github.com/DataDog/datadog-agent/comp/checks/windowseventlog/impl/check"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies for the windowseventlog component.
type Requires struct {
	Lifecycle compdef.Lifecycle
	// Logs Agent component, used to send integration logs
	// It is optional because the Logs Agent can be disabled
	LogsComponent          option.Option[logsAgent.Component]
	Config                 configComponent.Component
	PublisherMetadataCache publishermetadatacache.Component
}

// Provides defines the output of the windowseventlog component.
type Provides struct {
	Comp windowseventlog.Component
}

// NewComponent creates a new windowseventlog component.
func NewComponent(reqs Requires) Provides {
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			core.RegisterCheck(check.CheckName, check.Factory(reqs.LogsComponent, reqs.Config, reqs.PublisherMetadataCache))
			return nil
		},
	})
	return Provides{Comp: struct{}{}}
}
