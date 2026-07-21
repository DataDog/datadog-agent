// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package agent

import (
	"testing"

	jmxlogger "github.com/DataDog/datadog-agent/comp/agent/jmxlogger/def"
	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	defaultforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/mock"
	eventplatformmock "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/mock"
	orchestratormock "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t,
		Bundle(jmxlogger.NewDefaultParams()),
		core.MockBundle(),
		defaultforwardermock.MockModule(),
		orchestratormock.MockModule(),
		eventplatformmock.MockModule(),
		demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
}
