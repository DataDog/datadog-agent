// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package connectionscheckimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	"github.com/DataDog/datadog-agent/comp/process/connectionscheck"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestConnectionsCheckDisabledOnDarwin(t *testing.T) {
	sysprobeConfigs := map[string]interface{}{
		"network_config.enabled": true,
	}

	c := fxutil.Test[connectionscheck.Component](t, fx.Options(
		core.MockBundle(),
		fx.Replace(sysprobeconfigimpl.MockParams{Overrides: sysprobeConfigs}),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		npcollectorimpl.MockModule(),
		fx.Provide(func(t testing.TB) tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
		Module(),
	))

	assert.False(t, c.Object().IsEnabled())
}
