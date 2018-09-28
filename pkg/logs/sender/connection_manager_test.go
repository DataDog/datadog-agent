// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestAddress(t *testing.T) {
	connManager := NewConnectionManager(config.Endpoint{Host: "foo", Port: 1234})
	assert.Equal(t, "foo:1234", connManager.address())
}
