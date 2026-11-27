// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package fx

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logdef "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type remoteFlagSubscribers struct {
	fx.In

	Subscribers []pkgremoteflags.RemoteFlagSubscriber `group:"remoteFlagSubscriber"`
}

// The component's remote flag handlers reach the Remote Flags component
// through an fx group. That depends on fxutil.ProvideComponentConstructor
// preserving the `group` struct tag when it rewrites compdef.Out into fx.Out —
// a silent failure mode: the group would simply come back empty and every
// gated profile would stay off forever.
func TestModuleProvidesRemoteFlagSubscriber(t *testing.T) {
	got := fxutil.Test[remoteFlagSubscribers](t,
		fx.Options(
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			fx.Provide(func() logdef.Component { return logmock.New(t) }),
			telemetrymock.Module(),
			Module(),
		),
	)

	require.Len(t, got.Subscribers, 1)
	handlers := got.Subscribers[0].Handlers()
	require.NotEmpty(t, handlers)

	names := make([]pkgremoteflags.FlagName, 0, len(handlers))
	for _, h := range handlers {
		names = append(names, h.FlagName())
	}
	assert.Contains(t, names, pkgremoteflags.FlagName("troubleshooting_coat_bundle"))
	assert.Contains(t, names, pkgremoteflags.FlagName("agent_errortracking"))
}

// recordingLog records the info messages, to observe what gets built.
type recordingLog struct {
	logdef.Component
	mu   sync.Mutex
	msgs []string
}

func (l *recordingLog) Infof(format string, params ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, fmt.Sprintf(format, params...))
}

func (l *recordingLog) started() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, m := range l.msgs {
		if strings.HasPrefix(m, "Starting agent telemetry") {
			return true
		}
	}
	return false
}

// Binaries that only add the module, without injecting the component, must
// still get it built, started and subscribed to the remote flags.
func TestModuleBuildsComponentWhenNothingInjectsIt(t *testing.T) {
	rec := &recordingLog{Component: logmock.New(t)}

	fxutil.Test[struct{ fx.In }](t,
		fx.Options(
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			fx.Provide(func() logdef.Component { return rec }),
			telemetrymock.Module(),
			Module(),
		),
	)

	assert.True(t, rec.started())
}
