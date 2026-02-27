// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package networkpath

import (
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	traceroutemock "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/mock"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(),
		core.MockBundle(),
		hostnameimpl.MockModule(),
		eventplatformimpl.MockModule(),
		rdnsquerier.MockModule(),
		logscompression.MockModule(),
		fx.Provide(func() statsd.ClientInterface {
			return &statsd.NoOpClient{}
		}),
		fx.Provide(func(t testing.TB) traceroute.Component { return traceroutemock.Mock(t) }),
	)
}
