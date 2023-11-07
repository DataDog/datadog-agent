// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUpdateMinTracker(t *testing.T) {
	expiryDuration := 60 * time.Second

	mt := newMinTracker(expiryDuration)

	// Should update
	mt.update(10)
	assert.Equal(t, mt.get(), 10)

	// Should not update, since value didn't expire yet
	mt.update(11)
	assert.Equal(t, mt.get(), 10)

	// simulate waiting half the expirationDuration
	mt.timestamp = time.Now().Add(-expiryDuration / 2)

	// Should not update
	mt.update(199)
	assert.Equal(t, mt.get(), 10)

	// Shoud update, even if value didn't expire because new value is lower
	mt.update(5)
	assert.Equal(t, mt.get(), 5)

	// Change timestamp to simulate expiration
	mt.timestamp = time.Now().Add(-2 * expiryDuration)

	// Shoud update because current value has expired
	mt.update(100)
	assert.Equal(t, mt.get(), 100)
}
