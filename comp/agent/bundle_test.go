// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package agent

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger/jmxloggerimpl"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t,
		Bundle(),
		core.MockBundle(),
		compressionimpl.MockModule(),
		defaultforwarder.MockModule(),
		orchestratorimpl.MockModule(),
		eventplatformimpl.MockModule(),
		demultiplexerimpl.Module(),
		fx.Supply(demultiplexerimpl.NewDefaultParams()),
		fx.Supply(jmxloggerimpl.NewDefaultParams()))
}
