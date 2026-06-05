// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package aggregator

import (
	"testing"

	"go.uber.org/fx"

	demultiplexerimpl "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/impl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/fx-noop"
	filterlistfx "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	defaultforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/mock"
	eventplatformmock "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/mock"
	orchestratormock "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/mock"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(demultiplexerimpl.Params{}),
		core.MockBundle(),
		hostnameimpl.MockModule(),
		defaultforwardermock.MockModule(),
		orchestratormock.MockModule(),
		eventplatformmock.MockModule(),
		nooptagger.Module(),
		haagentmock.Module(),
		logscompression.MockModule(),
		metricscompression.MockModule(),
		filterlistfx.MockModule(),
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
	)
}
