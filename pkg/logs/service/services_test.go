// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddService(t *testing.T) {
	services := NewServices()
	service := NewService("foo", "1234", Before)

	services.AddService(service)

	added := services.GetAddedServicesForType("foo")
	assert.NotNil(t, added)
	assert.Equal(t, 0, len(added))

	go func() { services.AddService(service) }()
	assert.Equal(t, <-added, service)

	all := services.GetAllAddedServices()
	assert.NotNil(t, all)
	assert.Equal(t, 0, len(all))

	go func() { services.AddService(service) }()
	assert.Equal(t, <-added, service)
	assert.Equal(t, <-all, service)
}

func TestRemoveService(t *testing.T) {
	services := NewServices()
	service := NewService("foo", "1234", Before)

	services.RemoveService(service)

	removed := services.GetRemovedServicesForType("foo")
	assert.NotNil(t, removed)
	assert.Equal(t, 0, len(removed))

	go func() { services.RemoveService(service) }()
	assert.Equal(t, <-removed, service)

	all := services.GetAllRemovedServices()
	assert.NotNil(t, all)
	assert.Equal(t, 0, len(all))

	go func() { services.RemoveService(service) }()
	assert.Equal(t, <-removed, service)
	assert.Equal(t, <-all, service)
}
