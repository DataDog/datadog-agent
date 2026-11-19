// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build rc_x509 && test && ((linux && !android) || (darwin && !ios) || windows)

package fx

import (
	"testing"

	"github.com/stretchr/testify/require"
	uberfx "go.uber.org/fx"

	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	cfgcomp "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	rcx509client "github.com/DataDog/datadog-agent/comp/remote-config/rcx509client/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type fakeCollector struct{ collector.Component }

func (fakeCollector) GetChecks() []check.Check { return nil }

type handlerDeps struct {
	uberfx.In

	// Optional, like the component's own dependency: the test must tell
	// "not provided" apart from "provided but unresolved".
	Handler rcx509client.ExecuteHandler `optional:"true"`
}

func buildHandler(t *testing.T, overrides map[string]interface{}, withCollector bool) rcx509client.ExecuteHandler {
	t.Helper()
	opts := []uberfx.Option{
		Module(),
		uberfx.Provide(func() cfgcomp.Component { return cfgcomp.NewMockWithOverrides(t, overrides) }),
		uberfx.Provide(func() log.Component { return logmock.New(t) }),
	}
	if withCollector {
		opts = append(opts, uberfx.Provide(func() collector.Component { return fakeCollector{} }))
	}
	return fxutil.Test[handlerDeps](t, opts...).Handler
}

// The handler must reach the component through the real fx graph, not just
// through the constructor: a constructor that fx cannot resolve is silently
// replaced by nil for an optional dependency.
func TestModuleSuppliesExecuteHandlerOnlyWhenEnabled(t *testing.T) {
	enabled := map[string]interface{}{
		"remote_queries.enabled":                         true,
		"remote_queries.tunnel.execute_enabled":          true,
		"remote_queries.tunnel.org_id":                   620250,
		"remote_configuration.x509.report_connection_id": true,
	}
	t.Run("enabled", func(t *testing.T) {
		require.NotNil(t, buildHandler(t, enabled, true))
	})
	t.Run("default off", func(t *testing.T) {
		require.Nil(t, buildHandler(t, map[string]interface{}{"remote_queries.enabled": true}, true))
	})
	t.Run("enabled without org", func(t *testing.T) {
		require.Nil(t, buildHandler(t, map[string]interface{}{
			"remote_queries.tunnel.execute_enabled":          true,
			"remote_configuration.x509.report_connection_id": true,
		}, true))
	})
	t.Run("enabled without collector", func(t *testing.T) {
		require.Nil(t, buildHandler(t, enabled, false))
	})
}
