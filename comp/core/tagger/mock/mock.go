// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

// Package mock contains the implementation of the mock for the tagger component.
package mock

import (
	"net/http"
	"testing"

	"go.uber.org/fx"

	"github.com/stretchr/testify/assert"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Mock implements mock-specific methods for the tagger component.
type Mock interface {
	tagger.Component

	// SetTags allows to set tags in the mock fake tagger
	SetTags(entityID types.EntityID, source string, low, orch, high, std []string)

	// SetGlobalTags allows to set tags in store for the global entity
	SetGlobalTags(low, orch, high, std []string)
}

// mockTaggerClient is a mock of the tagger Component
type mockTaggerClient struct {
	*taggerimpl.TaggerWrapper
}

// mockHandleRequest is a simple mocked http.Handler function to test the route is registered correctly on the api component
func (m *mockTaggerClient) mockHandleRequest(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// New returns a Mock
func New(t testing.TB) Mock {
	c := configmock.New(t)
	params := tagger.Params{
		UseFakeTagger: true,
	}
	logComponent := logmock.New(t)
	wmeta := fxutil.Test[optional.Option[workloadmeta.Component]](t,
		fx.Provide(func() log.Component { return logComponent }),
		fx.Provide(func() config.Component { return c }),
		workloadmetafx.Module(workloadmeta.NewParams()),
	)

	tagger, err := taggerimpl.NewTaggerClient(params, c, wmeta, logComponent, noopTelemetry.GetCompatComponent())

	assert.NoError(t, err)

	return &mockTaggerClient{
		tagger,
	}
}

// Provides is a struct containing the mock and the endpoint
type Provides struct {
	fx.Out

	Comp     Mock
	Endpoint api.AgentEndpointProvider
}

type dependencies struct {
	fx.In

	Config    config.Component
	Log       log.Component
	WMeta     optional.Option[workloadmeta.Component]
	Telemetry telemetry.Component
}

// NewMock returns a Provides
func NewMock(deps dependencies) (Provides, error) {
	params := tagger.Params{
		UseFakeTagger: true,
	}

	tagger, err := taggerimpl.NewTaggerClient(params, deps.Config, deps.WMeta, deps.Log, deps.Telemetry)
	if err != nil {
		return Provides{}, err
	}

	c := &mockTaggerClient{
		tagger,
	}
	return Provides{
		Comp:     c,
		Endpoint: api.NewAgentEndpointProvider(c.mockHandleRequest, "/tagger-list", "GET"),
	}, nil
}

// Module is a module containing the mock, useful for testing
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock),
		fx.Supply(config.Params{}),
		fx.Supply(log.Params{}),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		config.MockModule(),
		sysprobeconfigimpl.MockModule(),
		workloadmetafx.Module(workloadmeta.NewParams()),
		telemetryimpl.MockModule(),
	)
}

// SetupFakeTagger calls fxutil.Test to create a mock tagger for testing
func SetupFakeTagger(t testing.TB) Mock {
	return fxutil.Test[Mock](t, Module())
}

// SetTags calls faketagger SetTags which sets the tags for an entity
func (m *mockTaggerClient) SetTags(entity types.EntityID, source string, low, orch, high, std []string) {
	if v, ok := m.TaggerWrapper.GetDefaultTagger().(*taggerimpl.FakeTagger); ok {
		v.SetTags(entity, source, low, orch, high, std)
	}
}

// SetGlobalTags calls faketagger SetGlobalTags which sets the tags for the global entity
func (m *mockTaggerClient) SetGlobalTags(low, orch, high, std []string) {
	if v, ok := m.TaggerWrapper.GetDefaultTagger().(*taggerimpl.FakeTagger); ok {
		v.SetGlobalTags(low, orch, high, std)
	}
}
