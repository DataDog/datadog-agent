// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestShouldReset(t *testing.T) {
	client := NewClient(10*time.Second, 0*time.Second)

	assert.False(t, client.shouldReset())
}
