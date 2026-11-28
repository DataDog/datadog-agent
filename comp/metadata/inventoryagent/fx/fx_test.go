// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package fx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameimpl "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type remoteFlagSubscribers struct {
	fx.In

	Subscribers []pkgremoteflags.RemoteFlagSubscriber `group:"remoteFlagSubscriber"`
}

// The remote flag handler reaches the Remote Flags component through an fx group, which relies on
// fxutil.ProvideComponentConstructor preserving the `group` struct tag.
func TestModuleProvidesRemoteFlagSubscriber(t *testing.T) {
	got := fxutil.Test[remoteFlagSubscribers](t,
		fx.Options(
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			fx.Provide(func() logdef.Component { return logmock.New(t) }),
			fx.Provide(func() sysprobeconfig.Component { return sysprobeconfigmock.NewMock(t) }),
			fxutil.ProvideOptional[sysprobeconfig.Component](),
			fx.Provide(func() serializer.MetricSerializer { return serializermock.NewMetricSerializer(t) }),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Provide(func(c ipc.Component) ipc.HTTPClient { return c.GetClient() }),
			hostnameimpl.MockModule(),
			Module(),
		),
	)

	require.Len(t, got.Subscribers, 1)
	handlers := got.Subscribers[0].Handlers()
	require.Len(t, handlers, 1)
	assert.Equal(t, pkgremoteflags.FlagName("inventory_drop_full_configuration"), handlers[0].FlagName())
}
