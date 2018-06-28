// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
)

func TestServiceToConfig(t *testing.T) {
	s := newStore()
	service := dummyService{
		ID:            "a5901276aed1",
		ADIdentifiers: []string{"redis"},
		Hosts:         map[string]string{"bridge": "127.0.0.1"},
	}
	s.addConfigForService(service.GetID(), integration.Config{Name: "foo"})
	s.addConfigForService(service.GetID(), integration.Config{Name: "bar"})
	assert.Equal(t, len(s.getConfigsForService(service.GetID())), 2)
	s.removeConfigsForService(service.GetID())
	s.addConfigForService(service.GetID(), integration.Config{Name: "foo"})
	assert.Equal(t, len(s.getConfigsForService(service.GetID())), 1)
}
