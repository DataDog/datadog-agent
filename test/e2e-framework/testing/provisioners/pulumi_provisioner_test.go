// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDumpRawResourcesRedactsSecrets(t *testing.T) {
	resources := RawResources{
		"dd-Host-aws-vm": []byte(`{"password":"super-secret-value"}`),
		"other-resource": []byte(`{"foo":"bar"}`),
	}
	secretKeys := map[string]bool{
		"dd-Host-aws-vm": true,
		"other-resource": false,
	}

	output := dumpRawResources(resources, secretKeys)

	assert.NotContains(t, output, "super-secret-value")
	assert.Contains(t, output, "dd-Host-aws-vm: [secret value redacted from logs]")
	assert.True(t, strings.Contains(output, `other-resource: {"foo":"bar"}`))
}
