// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
package statsd

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

func TestGet(t *testing.T) {
	s := fxutil.Test[Component](t, Module)
	c, err := s.GetForHostPort("127.0.0.1", 8125, ddgostatsd.WithoutTelemetry())
	assert.NoError(t, err)
	assert.NotNilf(t, c, "statsd client should not be nil")
	err = c.Close()
	assert.NoError(t, err)
}
