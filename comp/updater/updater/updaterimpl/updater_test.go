// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updaterimpl

import (
	"testing"

	"go.uber.org/fx"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type testDependencies struct {
	fx.In

	Dependencies dependencies
}

type mockLifecycle struct{}

func (m *mockLifecycle) Append(_ fx.Hook) {}

func TestUpdaterWithoutRemoteConfig(t *testing.T) {
	deps := fxutil.Test[testDependencies](t, fx.Options(
		core.MockBundle(),
		fx.Supply(core.BundleParams{}),
		fx.Supply(optional.NewNoneOption[rcservice.Component]()),
		Module(),
	))
	_, err := newUpdaterComponent(&mockLifecycle{}, deps.Dependencies)
	assert.ErrorIs(t, err, errRemoteConfigRequired)
}
