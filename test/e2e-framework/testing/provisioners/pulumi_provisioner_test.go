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
		"dd-Host-aws-vm": []byte(`{"address":"1.2.3.4","username":"Administrator","password":"super-secret-value"}`),
		"other-resource": []byte(`{"foo":"bar"}`),
	}
	secretKeys := map[string]bool{
		"dd-Host-aws-vm": true,
		"other-resource": false,
	}

	output := dumpRawResources(resources, secretKeys)

	assert.NotContains(t, output, "super-secret-value")
	assert.Contains(t, output, `"address": "1.2.3.4"`)
	assert.Contains(t, output, `"username": "Administrator"`)
	assert.Contains(t, output, `"password": "[redacted]"`)
	assert.True(t, strings.Contains(output, `other-resource: {"foo":"bar"}`))
}

func TestDumpRawResourcesRedactsWholeValueWhenNotAnObject(t *testing.T) {
	resources := RawResources{
		"secret-scalar": []byte(`"super-secret-value"`),
	}
	secretKeys := map[string]bool{
		"secret-scalar": true,
	}

	output := dumpRawResources(resources, secretKeys)

	assert.NotContains(t, output, "super-secret-value")
	assert.Contains(t, output, "secret-scalar: [secret value redacted from logs]")
}
