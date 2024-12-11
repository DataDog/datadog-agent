// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package certificate

import (
	"testing"
	"time"
)

func TestCreateSecretData(t *testing.T) {
	data, err := GenerateSecretData(time.Now(), time.Now().Add(1*time.Hour+1*time.Minute), []string{"my.dns.name"})
	if err != nil {
		t.Fatalf("Failed to create the Secret: %v", err)
	}

	_, err = ParseSecretData(data)
	if err != nil {
		t.Fatalf("Failed to parse the Secret: %v", err)
	}

	cert, err := GetCertFromSecret(data)
	if err != nil {
		t.Fatalf("Failed to parse the Secret: %v", err)
	}

	expiration := GetDurationBeforeExpiration(cert)
	if expiration < 1*time.Hour {
		t.Fatalf("The Secret expires too soon: %v", expiration)
	}
}
