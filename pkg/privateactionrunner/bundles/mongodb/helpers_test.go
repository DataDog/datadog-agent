// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_mongodb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetConnectionURITLS(t *testing.T) {
	tests := []struct {
		name string
		tls  string
		want string
	}{
		{
			name: "TLS enabled",
			tls:  "true",
			want: "mongodb://user:password@localhost:27017/?tls=true",
		},
		{
			name: "TLS disabled",
			tls:  "false",
			want: "mongodb://user:password@localhost:27017/?tls=false",
		},
		{
			name: "TLS defaults to enabled",
			want: "mongodb://user:password@localhost:27017/?tls=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentials := map[string]string{
				"username": "user",
				"password": "password",
				"host":     "localhost",
				"port":     "27017",
				"tls":      tt.tls,
			}

			got, err := getConnectionUri(credentials)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)

			clientOptions, _, err := createMongoClientOptions(context.Background(), credentials)
			require.NoError(t, err)
			require.NotNil(t, clientOptions)
		})
	}
}

func TestGetConnectionURIWithTLSAndAuthenticationOptions(t *testing.T) {
	credentials := map[string]string{
		"username":      "user",
		"password":      "password",
		"host":          "localhost",
		"port":          "27017",
		"database":      "admin",
		"authSource":    "users",
		"authMechanism": "SCRAM-SHA-256",
		"tls":           "true",
	}

	got, err := getConnectionUri(credentials)
	require.NoError(t, err)
	want := "mongodb://user:password@localhost:27017/admin?authSource=users&authMechanism=SCRAM-SHA-256&tls=true"
	require.Equal(t, want, got)
}
