// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestNewBearerTokenAuth(t *testing.T) {
	token := "test-token"
	auth := NewBearerTokenAuth(token)

	require.NotNil(t, auth)

	// Test GetRequestMetadata
	md, err := auth.GetRequestMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-token", md["authorization"])

	// Test RequireTransportSecurity
	assert.True(t, auth.RequireTransportSecurity())
}

func TestBearerTokenAuth_GetRequestMetadata(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "simple token",
			token:    "abc123",
			expected: "Bearer abc123",
		},
		{
			name:     "empty token",
			token:    "",
			expected: "Bearer ",
		},
		{
			name:     "token with special chars",
			token:    "abc-123_XYZ",
			expected: "Bearer abc-123_XYZ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewBearerTokenAuth(tt.token)
			md, err := auth.GetRequestMetadata(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.expected, md["authorization"])
		})
	}
}

func TestStaticAuthInterceptor_ValidToken(t *testing.T) {
	token := "valid-token"
	authFunc := StaticAuthInterceptor(token)

	// Create a context with the correct bearer token
	md := metadata.Pairs("authorization", "Bearer valid-token")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	newCtx, err := authFunc(ctx)
	require.NoError(t, err)
	assert.NotNil(t, newCtx)
}

func TestStaticAuthInterceptor_InvalidToken(t *testing.T) {
	token := "valid-token"
	authFunc := StaticAuthInterceptor(token)

	// Create a context with an incorrect bearer token
	md := metadata.Pairs("authorization", "Bearer invalid-token")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := authFunc(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid auth token")
}

func TestStaticAuthInterceptor_MissingToken(t *testing.T) {
	token := "valid-token"
	authFunc := StaticAuthInterceptor(token)

	// Create a context without authorization header
	ctx := context.Background()

	_, err := authFunc(ctx)
	assert.Error(t, err)
}

func TestAuthInterceptor_CustomVerifier(t *testing.T) {
	verifier := func(token string) (interface{}, error) {
		if token == "secret" {
			return "user-info", nil
		}
		return nil, assert.AnError
	}

	authFunc := AuthInterceptor(verifier)

	t.Run("valid token", func(t *testing.T) {
		md := metadata.Pairs("authorization", "Bearer secret")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		newCtx, err := authFunc(ctx)
		require.NoError(t, err)
		assert.NotNil(t, newCtx)
	})

	t.Run("invalid token", func(t *testing.T) {
		md := metadata.Pairs("authorization", "Bearer wrong")
		ctx := metadata.NewIncomingContext(context.Background(), md)

		_, err := authFunc(ctx)
		assert.Error(t, err)
	})
}
