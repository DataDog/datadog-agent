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

func TestStartupCatalog(t *testing.T) {
	cat := newCatalog()
	cat.startup = true
	token := cat.register("test1")

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
