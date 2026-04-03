// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package updaterimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type mockLifecycle struct{}

func (m *mockLifecycle) Append(_ compdef.Hook) {}

func TestUpdaterWithoutRemoteConfig(t *testing.T) {
	_, err := NewComponent(Requires{
		Lifecycle:    &mockLifecycle{},
		RemoteConfig: option.None[rcservice.Component](),
	})
	assert.ErrorIs(t, err, errRemoteConfigRequired)
}