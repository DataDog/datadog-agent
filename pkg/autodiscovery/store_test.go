// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

func countConfigsForTemplate(s *store, template string) int {
	return len(s.templateToConfigs[template])
}

func countConfigsForService(s *store, service string) int {
	return len(s.serviceToConfigs[service])
}

func TestServiceToConfig(t *testing.T) {
	s := newStore()
	service := dummyService{
		ID:            "a5901276aed1",
		ADIdentifiers: []string{"redis"},
		Hosts:         map[string]string{"bridge": "127.0.0.1"},
	}
	s.addConfigForService(service.GetServiceID(), integration.Config{Name: "foo"})
	s.addConfigForService(service.GetServiceID(), integration.Config{Name: "bar"})
	assert.Equal(t, countConfigsForService(s, service.GetServiceID()), 2)
	s.removeConfigsForService(service.GetServiceID())
	s.addConfigForService(service.GetServiceID(), integration.Config{Name: "foo"})
	assert.Equal(t, countConfigsForService(s, service.GetServiceID()), 1)
}

func TestTemplateToConfig(t *testing.T) {
	s := newStore()
	s.addConfigForTemplate("digest1", integration.Config{Name: "foo"})
	s.addConfigForTemplate("digest1", integration.Config{Name: "bar"})
	s.addConfigForTemplate("digest2", integration.Config{Name: "foo"})

	assert.Equal(t, countConfigsForTemplate(s, "digest1"), 2)
	assert.Equal(t, countConfigsForTemplate(s, "digest2"), 1)

	s.removeConfigsForTemplate("digest1")
	assert.Equal(t, countConfigsForTemplate(s, "digest1"), 0)
	assert.Equal(t, countConfigsForTemplate(s, "digest2"), 1)

	s.addConfigForTemplate("digest1", integration.Config{Name: "foo"})
	assert.Equal(t, countConfigsForTemplate(s, "digest1"), 1)
	assert.Equal(t, countConfigsForTemplate(s, "digest2"), 1)
}

func TestRemoveServiceForADID(t *testing.T) {
	store := newStore()
	svc1 := dummyService{
		ID:            "foo",
		ADIdentifiers: []string{"redis", "docker://foo"},
		Hosts:         map[string]string{"bridge": "127.0.0.1"},
	}

	svc2 := dummyService{
		ID:            "bar",
		ADIdentifiers: []string{"redis", "docker://bar"},
		Hosts:         map[string]string{"bridge": "127.0.0.1"},
	}

	store.setADIDForServices("redis", svc1.GetServiceID())
	store.setADIDForServices("docker://foo", svc1.GetServiceID())

	store.setADIDForServices("redis", svc2.GetServiceID())
	store.setADIDForServices("docker://bar", svc2.GetServiceID())

	store.removeServiceForADID(svc1.GetServiceID(), svc1.ADIdentifiers)

	entities, found := store.getServiceEntitiesForADID("redis")
	assert.True(t, found)
	assert.EqualValues(t, map[string]struct{}{"bar": {}}, entities)

	entities, found = store.getServiceEntitiesForADID("docker://bar")
	assert.True(t, found)
	assert.EqualValues(t, map[string]struct{}{"bar": {}}, entities)

	_, found = store.getServiceEntitiesForADID("docker://foo")
	assert.False(t, found)
}

func TestIDsOfChecksWithSecrets(t *testing.T) {
	testStore := newStore()

	testStore.setIDsOfChecksWithSecrets(map[checkid.ID]checkid.ID{
		"id1": "id2",
		"id3": "id4",
	})

	assert.Equal(t, checkid.ID("id2"), testStore.getIDOfCheckWithEncryptedSecrets("id1"))
	assert.Equal(t, checkid.ID("id4"), testStore.getIDOfCheckWithEncryptedSecrets("id3"))
	assert.Empty(t, testStore.getIDOfCheckWithEncryptedSecrets("non-existing"))

	testStore.deleteMappingsOfCheckIDsWithSecrets([]checkid.ID{"id1"})
	assert.Empty(t, testStore.getIDOfCheckWithEncryptedSecrets("id1"))

	testStore.deleteMappingsOfCheckIDsWithSecrets([]checkid.ID{"id3"})
	assert.Empty(t, testStore.getIDOfCheckWithEncryptedSecrets("id3"))
}
