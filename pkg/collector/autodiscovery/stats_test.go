// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

func TestNewAcErrorStats(t *testing.T) {
	s := newAcErrorStats()
	assert.Len(t, s.config, 0)
	assert.Len(t, s.loader, 0)
	assert.Len(t, s.run, 0)
}

func TestSetConfigError(t *testing.T) {
	s := newAcErrorStats()
	name := "foo.yaml"
	s.setConfigError(name, "anError")
	s.setConfigError(name, "anotherError")

	assert.Len(t, s.config, 1)
	assert.Equal(t, s.config[name], "anotherError")
}

func TestRemoveConfigError(t *testing.T) {
	s := newAcErrorStats()
	name := "foo.yaml"
	s.setConfigError(name, "anError")
	s.removeConfigError(name)
	assert.Len(t, s.config, 0)
}

func TestGetConfigErrors(t *testing.T) {
	s := newAcErrorStats()
	name := "foo.yaml"
	s.setConfigError(name, "anError")
	err := s.getConfigErrors()

	assert.Len(t, err, 1)
}

func TestSetLoaderError(t *testing.T) {
	s := newAcErrorStats()
	s.setLoaderError("aCheck", "aLoader", "anError")
	s.setLoaderError("anotherCheck", "aLoader", "anError")

	assert.Len(t, s.loader, 2) // 2 checks for this loader
	assert.Len(t, s.loader["aCheck"], 1)
	assert.Len(t, s.loader["anotherCheck"], 1)
}

func TestRemoveLoaderErrors(t *testing.T) {
	s := newAcErrorStats()
	s.setLoaderError("aCheck", "aLoader", "anError")
	s.removeLoaderErrors("aCheck")

	assert.Len(t, s.loader, 0)
}

func TestGetLoaderErrors(t *testing.T) {
	s := newAcErrorStats()
	s.setLoaderError("aCheck", "aLoader", "anError")
	errs := s.getLoaderErrors()
	assert.Len(t, errs, 1)
}

func TestSetRunError(t *testing.T) {
	s := newAcErrorStats()
	id := check.ID("fooID")
	s.setRunError(id, "anError")
	s.setRunError(id, "anotherError")

	assert.Len(t, s.run, 1)
	assert.Equal(t, s.run[id], "anotherError")
}

func TestRemoveRunError(t *testing.T) {
	s := newAcErrorStats()
	id := check.ID("fooID")
	s.setRunError(id, "anError")
	s.removeRunError(id)
	assert.Len(t, s.run, 0)
}

func TestGetRunError(t *testing.T) {
	s := newAcErrorStats()
	id := check.ID("fooID")
	s.setRunError(id, "anError")
	err := s.getRunErrors()

	assert.Len(t, err, 1)
}
