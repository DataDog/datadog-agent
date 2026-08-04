// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"strings"
	"testing"
)

func TestDumpRawResourcesRedactsPasswords(t *testing.T) {
	resources := RawResources{
		"dd-Host-aws-vm": []byte(`{
			"address": "10.0.0.1",
			"password": "top-secret-password",
			"nested": {
				"adminPassword": "nested-secret-password",
				"username": "Administrator"
			}
		}`),
	}

	output := dumpRawResources(resources)

	for _, secret := range []string{"top-secret-password", "nested-secret-password"} {
		if strings.Contains(output, secret) {
			t.Fatalf("dumpRawResources leaked secret %q in output:\n%s", secret, output)
		}
	}
	for _, expected := range []string{
		`"password": "<redacted>"`,
		`"adminPassword": "<redacted>"`,
		`"username": "Administrator"`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("dumpRawResources output does not contain %q:\n%s", expected, output)
		}
	}

	if !strings.Contains(string(resources["dd-Host-aws-vm"]), "top-secret-password") {
		t.Fatal("dumpRawResources modified the raw resources used for environment imports")
	}
}
