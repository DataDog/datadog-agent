// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updaterimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
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
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			"remote_updates":               true,
			"remote_configuration.enabled": true,
		}),
	})
	assert.EqualError(t, err, "remote config failed to initialize")
}
