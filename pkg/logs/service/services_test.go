// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddService(t *testing.T) {
	services := NewServices()

	service1 := NewService("foo", "one")
	services.AddService(service1)

	added := services.GetAddedServicesForType("foo")
	assert.NotNil(t, added)
	assert.Equal(t, <-added, service1)

	service2 := NewService("foo", "two")
	go func() { services.AddService(service2) }()
	assert.Equal(t, <-added, service2)

	all := services.GetAllAddedServices()
	assert.NotNil(t, all)
	{
		// order of the catch-up services is not defined
		s1 := <-all
		s2 := <-all
		assert.ElementsMatch(t, []*Service{s1, s2}, []*Service{service1, service2})
	}

	service3 := NewService("foo", "three")
	go func() { services.AddService(service3) }()
	assert.Equal(t, <-added, service3)
	assert.Equal(t, <-all, service3)
}

func TestRemoveService(t *testing.T) {
	services := NewServices()
	service := NewService("foo", "1234")

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

func TestRemoveMultipleService(t *testing.T) {
	services := NewServices()
	service1 := NewService("foo", "1")
	service2 := NewService("foo", "2")
	service3 := NewService("foo", "3")

	removed := services.GetRemovedServicesForType("foo")
	assert.NotNil(t, removed)
	assert.Equal(t, 0, len(removed))

	services.AddService(service1)
	services.AddService(service2)
	services.AddService(service3)
	services.AddService(service1)

	assert.Len(t, services.services, 4)

	go func() { services.RemoveService(service1) }()
	assert.Equal(t, <-removed, service1)

	assert.Len(t, services.services, 2)

	go func() { services.RemoveService(service1) }()
	assert.Equal(t, <-removed, service1)

	assert.Len(t, services.services, 2)
}
