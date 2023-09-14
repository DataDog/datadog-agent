// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package aggregator

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		// instantiate all of the components, since this is not done
		// automatically.
		fx.Invoke(func(demultiplexer.Component) {}),
		fx.Supply(demultiplexer.Params{}),
		Bundle,
		log.Module,
		fx.Supply(log.Params{}),
		config.Module,
		fx.Supply(config.Params{}),
		defaultforwarder.Module,
		fx.Supply(defaultforwarder.Params{}),
	))
}
