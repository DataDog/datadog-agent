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
