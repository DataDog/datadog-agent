// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
package statsd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCreate(t *testing.T) {
	s := fxutil.Test[Component](t, Module())
	c, err := s.CreateForHostPort("127.0.0.1", 8125, ddgostatsd.WithoutTelemetry())
	assert.NoError(t, err)
	assert.NotNilf(t, c, "statsd client should not be nil")
	err = c.Close()
	assert.NoError(t, err)
}

func TestGet(t *testing.T) {
	s := fxutil.Test[Component](t, Module())
	c, err := s.Get()
	assert.NoError(t, err)
	assert.NotNilf(t, c, "statsd client should not be nil")
	err = c.Close()
	assert.NoError(t, err)
}
