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

func TestIsSet(t *testing.T) {
	expiryDuration := 60 * time.Second
	mrr := newMinRemainingRequests(expiryDuration)

	// Should update
	mrr.update("10")
	assert.Equal(t, mrr.val, 10)

	// Should not update, since value didn't expire yet
	mrr.update("11")
	assert.Equal(t, mrr.val, 10)

	// Shoud update, even if value didn't expire because new value is lower
	mrr.update("5")
	assert.Equal(t, mrr.val, 5)

	// Change timestamp to simulate expiratio
	mrr.timestamp = time.Now().Add(-2 * expiryDuration)

	// Shoud update because current value has expired
	mrr.update("100")
	assert.Equal(t, mrr.val, 100)
}
