// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package http

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientShouldReset(t *testing.T) {
	client := NewClient(
		0,
		func() *http.Client {
			return &http.Client{}
		},
	)

	assert.False(t, client.shouldReset())
}
