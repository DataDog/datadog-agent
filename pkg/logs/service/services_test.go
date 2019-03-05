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

	added := services.GetAddedServices("foo")
	assert.NotNil(t, added)
	assert.Equal(t, 0, len(added))

	go func() { services.AddService(service) }()
	s := <-added
	assert.Equal(t, s, service)
}

func TestRemoveService(t *testing.T) {
	services := NewServices()
	service := NewService("foo", "1234", Before)

	services.RemoveService(service)

	removed := services.GetRemovedServices("foo")
	assert.NotNil(t, removed)
	assert.Equal(t, 0, len(removed))

	go func() { services.RemoveService(service) }()
	s := <-removed
	assert.Equal(t, s, service)
}
