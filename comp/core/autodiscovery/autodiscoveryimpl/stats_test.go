// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewAcErrorStats(t *testing.T) {
	s := newAcErrorStats()
	assert.Len(t, s.config, 0)
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
