// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"sync"

	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
)

// EndpointStatus is the live state of the autoscaling configuration endpoint
// client. It implements remoteconfig.StatusInstance, written by the service's
// goroutines and read by the status handler, so it owns a lock. It is created
// before the service exists, so both can share one object.
type EndpointStatus struct {
	mu    sync.RWMutex
	state endpointState
}

// endpointState is what the service has reported. started records that it is
// polling; reported, that it got an answer -- without the second, an unreachable
// endpoint is indistinguishable from a disabled organization, since both leave
// the fields below false.
type endpointState struct {
	started          bool
	reported         bool
	orgEnabled       bool
	apiKeyAuthorized bool
	lastError        string
}

// NewEndpointStatus returns the status handle shared by the autoscaling status
// section and the autoscaling Remote Configuration client.
func NewEndpointStatus() *EndpointStatus {
	return &EndpointStatus{}
}

var _ remoteconfig.StatusInstance = (*EndpointStatus)(nil)

// SetOrgEnabled records whether Remote Configuration is enabled for the target
// organization.
func (s *EndpointStatus) SetOrgEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.reported = true
	s.state.orgEnabled = enabled
}

// SetAPIKeyAuthorized records whether the configured API key carries the Remote
// Configuration Read permission.
func (s *EndpointStatus) SetAPIKeyAuthorized(authorized bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.reported = true
	s.state.apiKeyAuthorized = authorized
}

// SetLastError records the last configuration refresh error, or nil on success.
func (s *EndpointStatus) SetLastError(err error) {
	var message string
	if err != nil {
		message = err.Error()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.lastError = message
}

// MarkStarted records that the service behind this status is polling. It says
// nothing about the client, which the caller starts later.
func (s *EndpointStatus) MarkStarted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.started = true
}

// snapshot copies the state, so the renderer does not hold the lock while
// formatting.
func (s *EndpointStatus) snapshot() endpointState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}
