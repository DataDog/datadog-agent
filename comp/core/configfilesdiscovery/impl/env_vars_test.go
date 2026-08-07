// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSecretEnvVarName(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		want    bool
	}{
		{
			name:    "password",
			envName: "KAFKA_SSL_KEYSTORE_PASSWORD",
			want:    true,
		},
		{
			name:    "key material",
			envName: "KAFKA_SSL_KEYSTORE_KEY",
			want:    true,
		},
		{
			name:    "certificate material",
			envName: "KAFKA_SSL_TRUSTSTORE_CERTIFICATES",
			want:    true,
		},
		{
			name:    "jaas config",
			envName: "KAFKA_SASL_JAAS_CONFIG",
			want:    true,
		},
		{
			name:    "credentials",
			envName: "KAFKA_SASL_OAUTHBEARER_CLIENT_CREDENTIALS_CLIENT_SECRET",
			want:    true,
		},
		{
			name:    "license",
			envName: "CONFLUENT_LICENSE",
			want:    true,
		},
		{
			name:    "safe location reference",
			envName: "KAFKA_SSL_KEYSTORE_LOCATION",
		},
		{
			name:    "safe broker id",
			envName: "KAFKA_NODE_ID",
		},
		{
			name:    "token boundary",
			envName: "KAFKA_MONKEY_PATCH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsSecretEnvVarName(tt.envName))
		})
	}
}
