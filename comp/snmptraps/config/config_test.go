// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package config

import (
	"context"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"gopkg.in/yaml.v2"
)

const mockedHostname = "VeryLongHostnameThatDoesNotFitIntoTheByteArray"

var expectedEngineID = "\x80\xff\xff\xff\xff\x67\xb2\x0f\xe4\xdf\x73\x7a\xce\x28\x47\x03\x8f\x57\xe6\x5c\x98"

var expectedEngineIDs = map[string]string{
	"VeryLongHostnameThatDoesNotFitIntoTheByteArray": "\x80\xff\xff\xff\xff\x67\xb2\x0f\xe4\xdf\x73\x7a\xce\x28\x47\x03\x8f\x57\xe6\x5c\x98",
	"VeryLongHostnameThatIsDifferent":                "\x80\xff\xff\xff\xff\xe7\x21\xcc\xd7\x0b\xe1\x60\xc5\x18\xd7\xde\x17\x86\xb0\x7d\x36",
}

// structify converts any yamlizable object to a plain map[string]any
func structify[T any](obj T) (map[string]any, error) {
	out, err := yaml.Marshal(obj)
	if err != nil {
		return nil, err
	}
	result := make(map[string]any)
	err = yaml.Unmarshal(out, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

var usersV3 = []UserV3{
	{
		Username:     "user",
		AuthKey:      "password",
		AuthProtocol: "MD5",
		PrivKey:      "password",
		PrivProtocol: "AES",
	},
	{
		Username:     "user",
		AuthKey:      "password",
		AuthProtocol: "SHA",
		PrivKey:      "password",
		PrivProtocol: "DES",
	},
	{
		Username:     "user2",
		AuthKey:      "password",
		AuthProtocol: "MD5",
		PrivKey:      "password",
		PrivProtocol: "AES",
	},
}

var usmUsers = []*gosnmp.UsmSecurityParameters{
	{
		UserName:                 "user",
		AuthenticationProtocol:   gosnmp.MD5,
		AuthenticationPassphrase: "password",
		PrivacyProtocol:          gosnmp.AES,
		PrivacyPassphrase:        "password",
	},
	{
		UserName:                 "user",
		AuthenticationProtocol:   gosnmp.SHA,
		AuthenticationPassphrase: "password",
		PrivacyProtocol:          gosnmp.DES,
		PrivacyPassphrase:        "password",
	},
	{
		UserName:                 "user2",
		AuthenticationProtocol:   gosnmp.MD5,
		AuthenticationPassphrase: "password",
		PrivacyProtocol:          gosnmp.AES,
		PrivacyPassphrase:        "password",
	},
}

// withConfig returns an fx Module providing the default datadog config
// overridden with the given network device namespace and traps configuration.
// In tests for other things, prefer to just inject the trapConfig rather than
// building and parsing an entire fake DD config.
func withConfig(t testing.TB, trapConfig *TrapsConfig, globalNamespace string) fx.Option {
	t.Helper()
	overrides := make(map[string]interface{})
	if globalNamespace != "" {
		overrides["network_devices.namespace"] = globalNamespace
	}
	if trapConfig != nil {
		rawTrapConfig, err := structify(trapConfig)
		require.NoError(t, err)
		overrides["network_devices.snmp_traps"] = rawTrapConfig
	}
	return fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: overrides}),
	)
}

func buildConfig(conf config.Component, hnService hostname.Component) (*TrapsConfig, error) {
	name, err := hnService.Get(context.Background())
	if err != nil {
		return nil, err
	}
	c, err := ReadConfig(name, conf)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// testOptions provides several fx options that multiple tests need
var testOptions = fx.Options(
	fx.Provide(buildConfig),
	hostnameimpl.MockModule(),
	fx.Replace(hostnameimpl.MockHostname(mockedHostname)),
	logimpl.MockModule(),
)

func TestFullConfig(t *testing.T) {
	deps := fxutil.Test[struct {
		fx.In
		Config *TrapsConfig
		Logger log.Component
	}](t,
		testOptions,
		withConfig(t, &TrapsConfig{
			Port:             1234,
			Users:            usersV3,
			BindHost:         "127.0.0.1",
			CommunityStrings: []string{"public"},
			StopTimeout:      12,
			Namespace:        "foo",
		}, ""),
	)
	config := deps.Config
	logger := deps.Logger
	assert.Equal(t, uint16(1234), config.Port)
	assert.Equal(t, 12, config.StopTimeout)
	assert.Equal(t, []string{"public"}, config.CommunityStrings)
	assert.Equal(t, "127.0.0.1", config.BindHost)
	assert.Equal(t, "foo", config.Namespace)
	assert.Equal(t, usersV3, config.Users)

	params, err := config.BuildSNMPParams(logger)
	assert.NoError(t, err)
	assert.Equal(t, uint16(1234), params.Port)
	assert.Equal(t, gosnmp.Version3, params.Version)
	assert.Equal(t, "udp", params.Transport)
	assert.NotNil(t, params.Logger)
	assert.Equal(t, gosnmp.UserSecurityModel, params.SecurityModel)
	assert.Equal(t, &gosnmp.UsmSecurityParameters{AuthoritativeEngineID: expectedEngineID}, params.SecurityParameters)

	table := gosnmp.NewSnmpV3SecurityParametersTable(params.Logger)
	for _, usmUser := range usmUsers {
		err := table.Add(usmUser.UserName, usmUser)
		assert.Nil(t, err)
	}
	var usmConfigTests = []struct {
		name       string
		identifier string
	}{
		{
			"identifier: user has 2 entries",
			"user",
		},
		{
			"identifier: user2 has 1 entry",
			"user2",
		},
	}
	for _, usmConfigTest := range usmConfigTests {
		// Compare the security params after initializing the security keys (happens in the add to table)
		expected, _ := table.Get(usmConfigTest.identifier)
		actual, _ := params.TrapSecurityParametersTable.Get(usmConfigTest.identifier)
		assert.ElementsMatch(t, expected, actual)
	}
}

func TestMinimalConfig(t *testing.T) {
	deps := fxutil.Test[struct {
		fx.In
		Config *TrapsConfig
		Logger log.Component
	}](t,
		config.MockModule(),
		testOptions,
	)
	config := deps.Config
	logger := deps.Logger
	assert.Equal(t, uint16(9162), config.Port)
	assert.Equal(t, 5, config.StopTimeout)
	assert.Empty(t, config.CommunityStrings)
	assert.Equal(t, "0.0.0.0", config.BindHost)
	assert.Empty(t, config.Users)
	assert.Equal(t, "default", config.Namespace)

	params, err := config.BuildSNMPParams(logger)
	assert.NoError(t, err)
	assert.Equal(t, uint16(9162), params.Port)
	assert.Equal(t, gosnmp.Version2c, params.Version)
	assert.Equal(t, "udp", params.Transport)
	assert.NotNil(t, params.Logger)
	assert.Equal(t, nil, params.SecurityParameters)
}

func TestDefaultUsers(t *testing.T) {
	config := fxutil.Test[*TrapsConfig](t,
		testOptions,
		withConfig(t, &TrapsConfig{
			CommunityStrings: []string{"public"},
			StopTimeout:      11,
		}, ""),
	)
	assert.Equal(t, 11, config.StopTimeout)
}

func TestBuildAuthoritativeEngineID(t *testing.T) {
	for name, engineID := range expectedEngineIDs {
		config := fxutil.Test[*TrapsConfig](t,
			config.MockModule(),
			fx.Provide(buildConfig),
			hostnameimpl.MockModule(),
			fx.Replace(hostnameimpl.MockHostname(name)),
		)
		assert.Equal(t, engineID, config.authoritativeEngineID)
	}
}

func TestNamespaceIsNormalized(t *testing.T) {
	config := fxutil.Test[*TrapsConfig](t,
		testOptions,
		withConfig(t, &TrapsConfig{
			Namespace: "><\n\r\tfoo",
		}, ""),
	)
	assert.Equal(t, "--foo", config.Namespace)
}

func TestInvalidNamespace(t *testing.T) {
	ddConfig := fxutil.Test[config.Component](t,
		withConfig(t, &TrapsConfig{
			Namespace: strings.Repeat("x", 101),
		}, ""))
	_, err := ReadConfig("", ddConfig)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too long")
}

func TestNamespaceSetGlobally(t *testing.T) {
	config := fxutil.Test[*TrapsConfig](t,
		testOptions,
		withConfig(t, nil, "foo"),
	)
	assert.Equal(t, "foo", config.Namespace)
}

func TestNamespaceSetBothGloballyAndLocally(t *testing.T) {
	config := fxutil.Test[*TrapsConfig](t,
		testOptions,
		withConfig(t,
			&TrapsConfig{Namespace: "bar"},
			"foo"),
	)

	assert.Equal(t, "bar", config.Namespace)
}
