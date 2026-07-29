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
		envName string
		want    bool
	}{
		{envName: "KAFKA_SSL_KEYSTORE_PASSWORD", want: true},
		{envName: "KAFKA_SSL_KEYSTORE_KEY", want: true},
		{envName: "KAFKA_SSL_TRUSTSTORE_CERTIFICATES", want: true},
		{envName: "KAFKA_SASL_JAAS_CONFIG", want: true},
		{envName: "KAFKA_SASL_OAUTHBEARER_CLIENT_CREDENTIALS_CLIENT_SECRET", want: true},
		{envName: "CONFLUENT_LICENSE", want: true},
		{envName: "KAFKA_CFG_OAUTH_ACCESS_TOKEN", want: true},
		{envName: "CONFLUENT_API_TOKEN", want: true},
		{envName: "KAFKA_SSL_KEYSTORE_LOCATION"},
		{envName: "KAFKA_SASL_OAUTHBEARER_TOKEN_ENDPOINT_URL"},
		{envName: "KAFKA_NODE_ID"},
		{envName: "KAFKA_MONKEY_PATCH"},
	}

	for _, tt := range tests {
		t.Run(tt.envName, func(t *testing.T) {
			assert.Equal(t, tt.want, IsSecretEnvVarName(tt.envName))
		})
	}
}
