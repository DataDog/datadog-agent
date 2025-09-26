// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	workloadfiltermock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/mock"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

import "C"

func testIsContainerExcluded(t *testing.T) {
	sender := mocksender.NewMockSender("testID")
	logReceiver := option.None[integrations.Component]()
	tagger := nooptagger.NewComponent()
	filterStore := fxutil.Test[workloadfiltermock.Mock](t, fx.Options(
		fx.Provide(func() config.Component {
			mockConfig := config.NewMock(t)
			mockConfig.SetWithoutSource("container_exclude", []string{"image:bar", "kube_namespace:black"})
			mockConfig.SetWithoutSource("container_include", []string{"kube_namespace:white"})
			return mockConfig
		}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		noopTelemetry.Module(),
		workloadfilterfxmock.MockModule(),
	))
	scopeInitCheckContext(sender.GetSenderManager(), logReceiver, tagger, filterStore)

	assert.Equal(t, C.int(1), IsContainerExcluded(C.CString("foo"), C.CString("bar"), C.CString("ns")))
	assert.Equal(t, C.int(0), IsContainerExcluded(C.CString("foo"), C.CString("bar"), C.CString("white")))
	assert.Equal(t, C.int(1), IsContainerExcluded(C.CString("foo"), C.CString("baz"), C.CString("black")))
	assert.Equal(t, C.int(0), IsContainerExcluded(C.CString("foo"), C.CString("baz"), nil))
}
