// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the publishermetadatacache component
package mock

import (
	"testing"

	"go.uber.org/fx"

	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// PublisherMetadataCache is a mock implementation of the publishermetadatacache Component
type PublisherMetadataCache struct{}

// Module defines the fx options for the mock component
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() publishermetadatacache.Component {
			return &PublisherMetadataCache{}
		}))
}

// New returns a mock for publishermetadatacache component.
func New(_ testing.TB) publishermetadatacache.Component {
	return &PublisherMetadataCache{}
}

// Get implements the Component interface
func (m *PublisherMetadataCache) Get(_ string, _ evtapi.EventRecordHandle) (evtapi.EventPublisherMetadataHandle, error) {
	return evtapi.EventPublisherMetadataHandle(12345), nil
}
