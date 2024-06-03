// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package taggerimpl

import (
	"net/http"
	"testing"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/local"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// MockTaggerClient is a mock of the tagger Component
type MockTaggerClient struct {
	*TaggerClient
}

// mockHandleRequest is a simple mocked http.Handler function to test the route is registered correctly on the api component
func (m *MockTaggerClient) mockHandleRequest(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// MockProvides is a mock of the tagger.Component provides struct to test endpoints register properly
type MockProvides struct {
	fx.Out

	Comp     tagger.Mock
	Endpoint api.AgentEndpointProvider
}

var _ tagger.Component = (*MockTaggerClient)(nil)

// NewMock returns a MockTagger
func NewMock(deps dependencies) MockProvides {
	taggerClient := newTaggerClient(deps).Comp
	c := &MockTaggerClient{
		TaggerClient: taggerClient.(*TaggerClient),
	}
	return MockProvides{
		Comp:     c,
		Endpoint: api.NewAgentEndpointProvider(c.mockHandleRequest, "/tagger-list", "GET"),
	}
}

// MockModule is a module containing the mock, useful for testing
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewMock),
		fx.Supply(config.Params{}),
		fx.Supply(logimpl.Params{}),
		logimpl.MockModule(),
		config.MockModule(),
		sysprobeconfigimpl.MockModule(),
		fx.Supply(tagger.NewFakeTaggerParams()),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.Module(),
	)
}

// SetTags calls faketagger SetTags which sets the tags for an entity
func (m *MockTaggerClient) SetTags(entity, source string, low, orch, high, std []string) {
	if m.TaggerClient == nil {
		panic("Tagger must be initialized before calling SetTags")
	}
	if v, ok := m.TaggerClient.defaultTagger.(*local.FakeTagger); ok {
		v.SetTags(entity, source, low, orch, high, std)
	}
}

// SetupFakeTagger calls fxutil.Test to create a mock tagger for testing
func SetupFakeTagger(t *testing.T) tagger.Mock {
	return fxutil.Test[tagger.Mock](t, MockModule())
}

// ResetTagger resets the tagger
func (m *MockTaggerClient) ResetTagger() {
	tagger.UnlockGlobalTaggerClient()
}
