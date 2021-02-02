// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCertificates(t *testing.T) {
	certsConfig := &CertificatesConfig{
		Hosts:        "127.0.0.1,localhost",
		ValidFor:     time.Duration(24 * time.Hour),
		RsaBits:      1024,
		EcdsaCurve:   "",
		CertFilePath: "cert.pem",
		KeyFilePath:  "key.pem",
	}
	defer os.Remove(certsConfig.CertFilePath)
	defer os.Remove(certsConfig.KeyFilePath)

	err := GenerateCertificates(certsConfig)
	require.Nil(t, err)
	_, err = os.Stat(certsConfig.CertFilePath)
	assert.Nil(t, err)
	_, err = os.Stat(certsConfig.KeyFilePath)
	assert.Nil(t, err)
}
