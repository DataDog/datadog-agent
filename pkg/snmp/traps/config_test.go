// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

const mockedHostname = "VeryLongHostnameThatDoesNotFitIntoTheByteArray"

var expectedEngineID = "\x80\xff\xff\xff\xff\x67\xb2\x0f\xe4\xdf\x73\x7a\xce\x28\x47\x03\x8f\x57\xe6\x5c\x98"

func TestFullConfig(t *testing.T) {
	Configure(t, Config{
		Port: 1234,
		Users: []UserV3{
			{
				Username:     "user",
				AuthKey:      "password",
				AuthProtocol: "MD5",
				PrivKey:      "password",
				PrivProtocol: "AES",
			},
		},
		BindHost:         "127.0.0.1",
		CommunityStrings: []string{"public"},
		StopTimeout:      12,
	})
	config, err := ReadConfig(mockedHostname)
	assert.NoError(t, err)
	assert.Equal(t, uint16(1234), config.Port)
	assert.Equal(t, 12, config.StopTimeout)
	assert.Equal(t, []string{"public"}, config.CommunityStrings)
	assert.Equal(t, "127.0.0.1", config.BindHost)
	assert.Equal(t, []UserV3{
		{
			Username:     "user",
			AuthKey:      "password",
			AuthProtocol: "MD5",
			PrivKey:      "password",
			PrivProtocol: "AES",
		},
	}, config.Users)

	params, err := config.BuildSNMPParams()
	assert.NoError(t, err)
	assert.Equal(t, uint16(1234), params.Port)
	assert.Equal(t, gosnmp.Version3, params.Version)
	assert.Equal(t, "udp", params.Transport)
	assert.NotNil(t, params.Logger)
	assert.Equal(t, gosnmp.UserSecurityModel, params.SecurityModel)
	assert.Equal(t, &gosnmp.UsmSecurityParameters{
		UserName:                 "user",
		AuthoritativeEngineID:    expectedEngineID,
		AuthenticationProtocol:   gosnmp.MD5,
		AuthenticationPassphrase: "password",
		PrivacyProtocol:          gosnmp.AES,
		PrivacyPassphrase:        "password",
	}, params.SecurityParameters)
}

func TestMinimalConfig(t *testing.T) {
	Configure(t, Config{})
	config, err := ReadConfig("")
	assert.NoError(t, err)
	assert.Equal(t, uint16(162), config.Port)
	assert.Equal(t, 5, config.StopTimeout)
	assert.Equal(t, []string{}, config.CommunityStrings)
	assert.Equal(t, "localhost", config.BindHost)
	assert.Equal(t, []UserV3{}, config.Users)

	params, err := config.BuildSNMPParams()
	assert.NoError(t, err)
	assert.Equal(t, uint16(162), params.Port)
	assert.Equal(t, gosnmp.Version2c, params.Version)
	assert.Equal(t, "udp", params.Transport)
	assert.NotNil(t, params.Logger)
	assert.Equal(t, nil, params.SecurityParameters)
}

func TestDefaultUsers(t *testing.T) {
	Configure(t, Config{
		CommunityStrings: []string{"public"},
		StopTimeout:      11,
	})
	config, err := ReadConfig("")
	assert.NoError(t, err)

	assert.Equal(t, 11, config.StopTimeout)
}

func TestBuildAuthoritativeEngineID(t *testing.T) {
	Configure(t, Config{})
	config, err := ReadConfig(mockedHostname)
	assert.NoError(t, err)
	assert.Equal(t, expectedEngineID, config.authoritativeEngineID)
}
