// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package connectionscheckimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfigdef "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	npcollectormock "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/mock"
	connectionscheck "github.com/DataDog/datadog-agent/comp/process/connectionscheck/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestConnectionsCheckDisabledOnDarwin(t *testing.T) {
	sysprobeConf := sysprobeconfigmock.NewMockWithOverrides(t, map[string]interface{}{"network_config.enabled": true})

	c := fxutil.Test[connectionscheck.Component](t, fx.Options(
		fx.Provide(func(t testing.TB) config.Component { return config.NewMock(t) }),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func() sysprobeconfigdef.Component { return sysprobeConf }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		npcollectormock.MockModule(),
		fx.Provide(func(t testing.TB) tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
		fxutil.ProvideComponentConstructor(NewComponent),
	))

	assert.False(t, c.Object().IsEnabled())
}
