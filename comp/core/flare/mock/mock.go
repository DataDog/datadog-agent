// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package mock

import (
	"net/http"
	"testing"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	flare "github.com/DataDog/datadog-agent/comp/core/flare/def"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
)

type Provides struct {
	Comp     flare.Component
	Endpoint api.AgentEndpointProvider
}

// MockFlare is a mock of the
type MockFlare struct{}

// ServeHTTP is a simple mocked http.Handler function
func (fc *MockFlare) handlerFunc(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("OK"))
}

// Create mocks the flare create function
func (fc *MockFlare) Create(_ flaretypes.ProfileData, _ time.Duration, _ error, _ []byte) (string, error) {
	return "", nil
}

// Send mocks the flare send function
func (fc *MockFlare) Send(_ string, _ string, _ string, _ helpers.FlareSource) (string, error) {
	return "", nil
}

// CreateWithArgs mocks the flare create with args function
func (fc *MockFlare) CreateWithArgs(_ flaretypes.FlareArgs, _ time.Duration, _ error, _ []byte) (string, error) {
	return "", nil
}

// New returns a new flare provider
func New(t testing.TB) Provides {
	m := &MockFlare{}

	return Provides{
		Comp:     m,
		Endpoint: api.NewAgentEndpointProvider(m.handlerFunc, "/flare", "POST"),
	}
}
