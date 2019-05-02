// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package flare

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/stretchr/testify/assert"
)

func TestConfigCredentialsCleaning(t *testing.T) {
	testCases := []struct {
		name string
		in   integration.Data
		out  []byte
	}{
		{
			name: "empty data",
			in:   integration.Data(""),
			out:  []byte(""),
		},
		{
			name: "nominal case",
			in:   integration.Data("tags: [\"bar:foo\", \"foo:bar\"]"),
			out:  []byte("tags: [\"bar:foo\", \"foo:bar\"]"),
		},
		{
			name: "password case",
			in:   integration.Data("password: a-password"),
			out:  []byte("password: ********"),
		},
		{
			name: "key_store_password case",
			in:   integration.Data("key_store_password: a-password"),
			out:  []byte("key_store_password: ********"),
		},
		{
			name: "api_key case",
			in:   integration.Data("api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			out:  []byte("api_key: ***************************aaaaa"),
		},
		{
			name: "auth_token case",
			in:   integration.Data("auth_token: auth-token"),
			out:  []byte("auth_token: ********"),
		},
	}

	for i, test := range testCases {
		t.Run(fmt.Sprintf("case %d: %s", i, test.name), func(t *testing.T) {
			result := cleanCredentials(test.in)
			assert.Equal(t, test.out, result)
		})
	}
}
