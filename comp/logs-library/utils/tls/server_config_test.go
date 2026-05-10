// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tlsutil

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientAuthRequiresVerification(t *testing.T) {
	assert.False(t, ClientAuthRequiresVerification(tls.NoClientCert))
	assert.False(t, ClientAuthRequiresVerification(tls.RequestClientCert))
	assert.False(t, ClientAuthRequiresVerification(tls.RequireAnyClientCert))
	assert.True(t, ClientAuthRequiresVerification(tls.VerifyClientCertIfGiven))
	assert.True(t, ClientAuthRequiresVerification(tls.RequireAndVerifyClientCert))
}

func TestClientAuthNoVerify(t *testing.T) {
	assert.Equal(t, tls.RequestClientCert, clientAuthNoVerify(tls.VerifyClientCertIfGiven))
	assert.Equal(t, tls.RequireAnyClientCert, clientAuthNoVerify(tls.RequireAndVerifyClientCert))
	assert.Equal(t, tls.NoClientCert, clientAuthNoVerify(tls.NoClientCert))
	assert.Equal(t, tls.RequestClientCert, clientAuthNoVerify(tls.RequestClientCert))
	assert.Equal(t, tls.RequireAnyClientCert, clientAuthNoVerify(tls.RequireAnyClientCert))
}

func TestValidate(t *testing.T) {
	t.Run("valid with cert and key", func(t *testing.T) {
		c := &ServerConfig{CertFile: "/cert", KeyFile: "/key"}
		assert.NoError(t, c.Validate())
	})

	t.Run("valid with mutual auth", func(t *testing.T) {
		c := &ServerConfig{
			CertFile:   "/cert",
			KeyFile:    "/key",
			CAFile:     "/ca",
			ClientAuth: tls.RequireAndVerifyClientCert,
		}
		assert.NoError(t, c.Validate())
	})

	t.Run("missing key_file", func(t *testing.T) {
		c := &ServerConfig{CertFile: "/cert"}
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cert_file and key_file")
	})

	t.Run("missing cert_file", func(t *testing.T) {
		c := &ServerConfig{KeyFile: "/key"}
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cert_file and key_file")
	})

	t.Run("VerifyClientCertIfGiven without ca_file", func(t *testing.T) {
		c := &ServerConfig{
			CertFile:   "/cert",
			KeyFile:    "/key",
			ClientAuth: tls.VerifyClientCertIfGiven,
		}
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ca_file")
	})

	t.Run("RequireAndVerifyClientCert without ca_file", func(t *testing.T) {
		c := &ServerConfig{
			CertFile:   "/cert",
			KeyFile:    "/key",
			ClientAuth: tls.RequireAndVerifyClientCert,
		}
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ca_file")
	})

	t.Run("NoClientCert without ca_file is OK", func(t *testing.T) {
		c := &ServerConfig{CertFile: "/cert", KeyFile: "/key", ClientAuth: tls.NoClientCert}
		assert.NoError(t, c.Validate())
	})

	t.Run("VerifyClientCertIfGiven with ca_file is OK", func(t *testing.T) {
		c := &ServerConfig{
			CertFile:   "/cert",
			KeyFile:    "/key",
			CAFile:     "/ca",
			ClientAuth: tls.VerifyClientCertIfGiven,
		}
		assert.NoError(t, c.Validate())
	})
}
