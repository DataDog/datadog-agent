// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"testing"

	"github.com/soniah/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestConfigCommon(t *testing.T) {
	config := TrapListenerConfig{
		Port:      162,
		Community: "public",
	}
	params, err := config.BuildParams()
	assert.NoError(t, err)

	assert.Equal(t, "udp", params.Transport)
	assert.NotNil(t, params.Logger)
}

func TestConfigPort(t *testing.T) {
	t.Run("err-required", func(t *testing.T) {
		config := TrapListenerConfig{
			Community: "public",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Community: "public",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)
		assert.Equal(t, 162, int(params.Port))
	})
}

func TestConfigVersion(t *testing.T) {
	t.Run("infer-v2c", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Community: "public",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version2c, params.Version)
		assert.Equal(t, "public", params.Community)
		assert.Equal(t, 0, int(params.SecurityModel))
		assert.Nil(t, params.SecurityParameters)
	})

	t.Run("infer-v3", func(t *testing.T) {
		config := TrapListenerConfig{
			Port: 162,
			User: "doggo",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version3, params.Version)
		assert.Equal(t, "", params.Community)
		assert.Equal(t, gosnmp.UserSecurityModel, params.SecurityModel)
		assert.NotNil(t, params.SecurityParameters)
		sp := params.SecurityParameters.(*gosnmp.UsmSecurityParameters)
		assert.Equal(t, "doggo", sp.UserName)
		assert.Equal(t, 0, int(sp.AuthenticationProtocol))
		assert.Equal(t, "", sp.AuthenticationPassphrase)
		assert.Equal(t, 0, int(sp.PrivacyProtocol))
		assert.Equal(t, "", sp.PrivacyPassphrase)
	})

	t.Run("err-could-not-infer", func(t *testing.T) {
		config := TrapListenerConfig{
			Port: 162,
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("explicit-v1", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Version:   "1",
			Community: "public", // Included
			User:      "doggo",  // Ignored
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version1, params.Version)
		assert.Equal(t, "public", params.Community)
		assert.Equal(t, 0, int(params.SecurityModel))
		assert.Nil(t, params.SecurityParameters)
	})

	t.Run("explicit-2-alias-2c", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Version:   "2", // Convenience alias for '2c'
			Community: "public",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version2c, params.Version)
		assert.Equal(t, "public", params.Community)
		assert.Equal(t, 0, int(params.SecurityModel))
		assert.Nil(t, params.SecurityParameters)
	})

	t.Run("err-invalid-version", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:    162,
			Version: "42",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})
}

func TestConfigV2(t *testing.T) {
	t.Run("community", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Community: "public",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version2c, params.Version)
		assert.Equal(t, "public", params.Community)
		assert.Equal(t, 0, int(params.SecurityModel))
		assert.Nil(t, params.SecurityParameters)
	})

	t.Run("err-community-missing", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:    162,
			Version: "2c",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})
}

func TestConfigV3(t *testing.T) {
	t.Run("err-user-missing", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:    162,
			Version: "3",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("no-auth-no-priv", func(t *testing.T) {
		config := TrapListenerConfig{
			Port: 162,
			User: "doggo",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version3, params.Version)
		assert.Equal(t, "", params.Community)
		assert.Equal(t, gosnmp.UserSecurityModel, params.SecurityModel)
		assert.NotNil(t, params.SecurityParameters)
		sp := params.SecurityParameters.(*gosnmp.UsmSecurityParameters)
		assert.Equal(t, "doggo", sp.UserName)
		assert.Equal(t, 0, int(sp.AuthenticationProtocol))
		assert.Equal(t, "", sp.AuthenticationPassphrase)
		assert.Equal(t, 0, int(sp.PrivacyProtocol))
		assert.Equal(t, "", sp.PrivacyPassphrase)
	})

	t.Run("auth-no-priv", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:         162,
			User:         "doggo",
			AuthProtocol: "MD5",
			AuthKey:      "doggopass",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version3, params.Version)
		assert.Equal(t, "", params.Community)
		assert.Equal(t, gosnmp.UserSecurityModel, params.SecurityModel)
		assert.NotNil(t, params.SecurityParameters)
		sp := params.SecurityParameters.(*gosnmp.UsmSecurityParameters)
		assert.Equal(t, "doggo", sp.UserName)
		assert.Equal(t, gosnmp.MD5, sp.AuthenticationProtocol)
		assert.Equal(t, "doggopass", sp.AuthenticationPassphrase)
		assert.Equal(t, 0, int(sp.PrivacyProtocol))
		assert.Equal(t, "", sp.PrivacyPassphrase)
	})

	t.Run("auth-no-priv-err-missing-auth-protocol", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:    162,
			User:    "doggo",
			AuthKey: "doggopass",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("auth-no-priv-err-missing-auth-key", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:         162,
			User:         "doggo",
			AuthProtocol: "MD5",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("no-auth-priv-unsupported", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:         162,
			User:         "doggo",
			PrivProtocol: "DES",
			PrivKey:      "doggokey",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("auth-priv", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:         162,
			User:         "doggo",
			AuthProtocol: "SHA",
			AuthKey:      "doggopass",
			PrivProtocol: "AES",
			PrivKey:      "doggokey",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version3, params.Version)
		assert.Equal(t, "", params.Community)
		assert.Equal(t, gosnmp.UserSecurityModel, params.SecurityModel)
		assert.NotNil(t, params.SecurityParameters)
		sp := params.SecurityParameters.(*gosnmp.UsmSecurityParameters)
		assert.Equal(t, "doggo", sp.UserName)
		assert.Equal(t, gosnmp.SHA, sp.AuthenticationProtocol)
		assert.Equal(t, "doggopass", sp.AuthenticationPassphrase)
		assert.Equal(t, gosnmp.AES, sp.PrivacyProtocol)
		assert.Equal(t, "doggokey", sp.PrivacyPassphrase)
	})

	t.Run("auth-priv-err-missing-priv-protocol", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:         162,
			User:         "doggo",
			AuthProtocol: "SHA",
			AuthKey:      "doggopass",
			PrivKey:      "doggokey",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("auth-priv-err-missing-priv-key", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:         162,
			User:         "doggo",
			AuthProtocol: "SHA",
			AuthKey:      "doggopass",
			PrivProtocol: "AES",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("err-unknown-auth", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:         162,
			User:         "doggo",
			AuthProtocol: "whatever",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("err-unknown-priv", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:         162,
			User:         "doggo",
			AuthProtocol: "SHA",
			PrivProtocol: "whatever",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("has-logger", func(t *testing.T) {
		// A bug in GoSNMP requires SecurityParameters to have a logger - otherwise receiving a v3 trap would crash
		// (because GoSNMP will try to access the nil logger).
		config := TrapListenerConfig{
			Port: 162,
			User: "doggo",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.NotNil(t, params.SecurityParameters)
		sp := params.SecurityParameters.(*gosnmp.UsmSecurityParameters)
		assert.NotNil(t, sp.Logger)
	})
}
