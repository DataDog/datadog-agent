// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPassCredMetadata(t *testing.T) {
	cred := newPassCred("admin", "admin123", false)
	md, err := cred.GetRequestMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "admin", md["username"])
	assert.Equal(t, "admin123", md["password"])
	assert.Equal(t, "Basic YWRtaW46YWRtaW4xMjM=", md["authorization"])
	assert.False(t, cred.RequireTransportSecurity())
}

func TestTransportConfig(t *testing.T) {
	assert.Equal(t, TransportInsecure, TransportConfig{}.mode())
	assert.Equal(t, TransportTLS, TransportConfig{UseTLS: true}.mode())
}
