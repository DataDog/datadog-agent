// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !go1.23

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestInternLoadOrStoreReset(t *testing.T) {
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	assert := assert.New(t)
	stringInternerTelemetry := newSiTelemetry(false, telemetryComp)
	sInterner := newStringInterner(4, 1, stringInternerTelemetry)

	// first test that the good value is returned.
	sInterner.LoadOrStore([]byte("foo"))
	assert.Equal(1, sInterner.cacheSize())
	sInterner.LoadOrStore([]byte("bar"))
	sInterner.LoadOrStore([]byte("bar"))
	assert.Equal(2, sInterner.cacheSize())
	sInterner.LoadOrStore([]byte("boo"))
	assert.Equal(3, sInterner.cacheSize())
	sInterner.LoadOrStore([]byte("far"))
	sInterner.LoadOrStore([]byte("far"))
	sInterner.LoadOrStore([]byte("far"))
	assert.Equal(4, sInterner.cacheSize())
	sInterner.LoadOrStore([]byte("val"))
	assert.Equal(1, sInterner.cacheSize())
	sInterner.LoadOrStore([]byte("val"))
	assert.Equal(1, sInterner.cacheSize())
}