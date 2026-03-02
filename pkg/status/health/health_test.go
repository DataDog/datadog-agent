// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package health

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmptyCatalog(t *testing.T) {
	cat := newCatalog()

	status := cat.getStatus()
	assert.Len(t, status.Healthy, 0)
	assert.Len(t, status.Unhealthy, 0)
}

func TestCatalogStartsHealthy(t *testing.T) {
	cat := newCatalog()
	// Register a fake compoment
	// because without any registered component, the `healthcheck` component would be disabled
	_ = cat.register("test1")

	status := cat.getStatus()
	assert.Len(t, status.Healthy, 1)
	assert.Contains(t, status.Healthy, "healthcheck")
	assert.Len(t, status.Unhealthy, 1)
	assert.Contains(t, status.Unhealthy, "test1")
}

func TestCatalogGetsUnhealthyAndBack(t *testing.T) {
	cat := newCatalog()
	// Register a fake compoment
	// because without any registered component, the `healthcheck` component would be disabled
	_ = cat.register("test1")

	status := cat.getStatus()
	assert.Contains(t, status.Healthy, "healthcheck")

	cat.latestRun = time.Now().Add(-1 * time.Hour)
	status = cat.getStatus()
	assert.Contains(t, status.Unhealthy, "healthcheck")

	cat.latestRun = time.Now()
	status = cat.getStatus()
	assert.Contains(t, status.Healthy, "healthcheck")
}

func TestRegisterAndUnhealthy(t *testing.T) {
	cat := newCatalog()
	token := cat.register("test1")

	_, found := cat.components[token]
	require.True(t, found)

	status := cat.getStatus()
	assert.Len(t, status.Healthy, 1)
	assert.Len(t, status.Unhealthy, 1)
	assert.Contains(t, status.Unhealthy, "test1")
}

func TestRegisterTriplets(t *testing.T) {
	cat := newCatalog()
	cat.register("triplet")
	cat.register("triplet")
	cat.register("triplet")
	assert.Len(t, cat.components, 3)
}

func TestDeregister(t *testing.T) {
	cat := newCatalog()
	token1 := cat.register("test1")
	token2 := cat.register("test2")

	assert.Len(t, cat.components, 2)

	err := cat.deregister(token1)
	assert.NoError(t, err)
	assert.Len(t, cat.components, 1)
	assert.Contains(t, cat.components, token2)
}

func TestDeregisterBadToken(t *testing.T) {
	cat := newCatalog()
	token1 := cat.register("test1")

	assert.Len(t, cat.components, 1)

	err := cat.deregister(nil)
	assert.NotNil(t, err)
	assert.Len(t, cat.components, 1)
	assert.Contains(t, cat.components, token1)
}

func TestGetHealthy(t *testing.T) {
	cat := newCatalog()
	token := cat.register("test1")

	// Start unhealthy
	status := cat.getStatus()
	assert.Len(t, status.Healthy, 1)
	assert.Len(t, status.Unhealthy, 1)

	// Start responding, become healthy
	<-token.C
	cat.pingComponents(time.Time{})
	status = cat.getStatus()
	assert.Len(t, status.Healthy, 2)
	assert.Len(t, status.Unhealthy, 0)

	// Make sure we keep staying healthy
	for i := 1; i < 10; i++ {
		<-token.C
		cat.pingComponents(time.Time{})
	}
	status = cat.getStatus()
	assert.Len(t, status.Healthy, 2)
	assert.Len(t, status.Unhealthy, 0)
}

func TestCatalogWithOnceComponent(t *testing.T) {
	cat := newCatalog()
	token := cat.register("test1", Once)

	// Start unhealthy
	status := cat.getStatus()
	assert.Len(t, status.Healthy, 1)
	assert.Contains(t, status.Healthy, "healthcheck")
	assert.Len(t, status.Unhealthy, 1)
	assert.Contains(t, status.Unhealthy, "test1")

	// Get healthy
	<-token.C
	cat.pingComponents(time.Time{}) // First ping will make component healthy and fill the channel again.
	status = cat.getStatus()
	assert.Len(t, status.Healthy, 2)
	assert.Contains(t, status.Healthy, "test1")

	// Make sure that we stay health even if we don't ping.
	cat.pingComponents(time.Time{})
	status = cat.getStatus()
	assert.Len(t, status.Healthy, 2)
	assert.Contains(t, status.Healthy, "test1")
}

func TestMulDuration(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		x        int
		expected time.Duration
	}{
		{
			name:     "multiply by 2",
			d:        time.Second,
			x:        2,
			expected: 2 * time.Second,
		},
		{
			name:     "multiply by 0",
			d:        time.Second,
			x:        0,
			expected: 0,
		},
		{
			name:     "multiply minutes",
			d:        5 * time.Minute,
			x:        3,
			expected: 15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mulDuration(tt.d, tt.x)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRegisterReadiness(t *testing.T) {
	handle := RegisterReadiness("test-readiness")
	require.NotNil(t, handle)
	require.NotNil(t, handle.C)

	// Cleanup
	err := Deregister(handle)
	assert.NoError(t, err)
}

func TestRegisterLiveness(t *testing.T) {
	handle := RegisterLiveness("test-liveness")
	require.NotNil(t, handle)
	require.NotNil(t, handle.C)

	// Cleanup
	err := Deregister(handle)
	assert.NoError(t, err)
}

func TestRegisterStartup(t *testing.T) {
	handle := RegisterStartup("test-startup")
	require.NotNil(t, handle)
	require.NotNil(t, handle.C)

	// Cleanup
	err := Deregister(handle)
	assert.NoError(t, err)
}

func TestGlobalDeregister(t *testing.T) {
	// Test deregistering from readinessAndLivenessCatalog
	handle1 := RegisterLiveness("test-deregister-1")
	err := Deregister(handle1)
	assert.NoError(t, err)

	// Test deregistering from readinessOnlyCatalog
	handle2 := RegisterReadiness("test-deregister-2")
	err = Deregister(handle2)
	assert.NoError(t, err)

	// Test deregistering from startupOnlyCatalog
	handle3 := RegisterStartup("test-deregister-3")
	err = Deregister(handle3)
	assert.NoError(t, err)

	// Test deregistering non-existent handle
	fakeHandle := &Handle{}
	err = Deregister(fakeHandle)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestGetLive(t *testing.T) {
	handle := RegisterLiveness("test-getlive")
	defer Deregister(handle)

	status := GetLive()
	// Should have healthcheck and test-getlive (unhealthy initially)
	assert.Contains(t, status.Unhealthy, "test-getlive")
}

func TestGetReady(t *testing.T) {
	handle1 := RegisterLiveness("test-getready-live")
	handle2 := RegisterReadiness("test-getready-ready")
	defer Deregister(handle1)
	defer Deregister(handle2)

	status := GetReady()
	// Should include components from both catalogs
	allComponents := append(status.Healthy, status.Unhealthy...)
	assert.Contains(t, allComponents, "test-getready-live")
	assert.Contains(t, allComponents, "test-getready-ready")
}

func TestGetStartup(t *testing.T) {
	handle := RegisterStartup("test-getstartup")
	defer Deregister(handle)

	status := GetStartup()
	// Should have test-getstartup (unhealthy initially)
	assert.Contains(t, status.Unhealthy, "test-getstartup")
}

func TestGetLiveNonBlocking(t *testing.T) {
	handle := RegisterLiveness("test-nonblocking-live")
	defer Deregister(handle)

	status, err := GetLiveNonBlocking()
	assert.NoError(t, err)
	allComponents := append(status.Healthy, status.Unhealthy...)
	assert.Contains(t, allComponents, "test-nonblocking-live")
}

func TestGetReadyNonBlocking(t *testing.T) {
	handle := RegisterReadiness("test-nonblocking-ready")
	defer Deregister(handle)

	status, err := GetReadyNonBlocking()
	assert.NoError(t, err)
	allComponents := append(status.Healthy, status.Unhealthy...)
	assert.Contains(t, allComponents, "test-nonblocking-ready")
}

func TestGetStartupNonBlocking(t *testing.T) {
	handle := RegisterStartup("test-nonblocking-startup")
	defer Deregister(handle)

	status, err := GetStartupNonBlocking()
	assert.NoError(t, err)
	allComponents := append(status.Healthy, status.Unhealthy...)
	assert.Contains(t, allComponents, "test-nonblocking-startup")
}

func TestHandleDeregister(t *testing.T) {
	handle := RegisterLiveness("test-handle-deregister")
	err := handle.Deregister()
	assert.NoError(t, err)
}
