// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const dbmsTag = "dbms:oracle"

const doesNotExist = "does-not-exist"

const (
	useDefaultUser = iota
	useLegacyUser
	useDoesNotExistUser
	useSysUser
)

const (
	expectedSessionsDefault           = 2
	expectedSessionsWithCustomQueries = 3
)

func getConnectData(t *testing.T, userType int) config.ConnectionConfig {
	handleRealConnection := func(userType int) config.ConnectionConfig {
		var userEnvVariable string
		var passwordEnvVariable string

		serverEnvVariable := "ORACLE_TEST_SERVER"
		serviceNameEnvVariable := "ORACLE_TEST_SERVICE_NAME"
		portEnvVariable := "ORACLE_TEST_PORT"

		switch userType {
		case useDefaultUser:
			userEnvVariable = "ORACLE_TEST_USER"
			passwordEnvVariable = "ORACLE_TEST_PASSWORD"
		case useLegacyUser:
			userEnvVariable = "ORACLE_TEST_LEGACY_USER"
			passwordEnvVariable = "ORACLE_TEST_LEGACY_PASSWORD"
		case useSysUser:
			userEnvVariable = "ORACLE_TEST_SYS_USER"
			passwordEnvVariable = "ORACLE_TEST_SYS_PASSWORD"
		}

		server := os.Getenv(serverEnvVariable)
		if server == "" {
			server = "localhost"
		}
		serviceName := os.Getenv(serviceNameEnvVariable)
		if serviceName == "" {
			serviceName = "XE"
		}

		username := os.Getenv(userEnvVariable)
		password := os.Getenv(passwordEnvVariable)
		if username == "" {
			switch userType {
			case useDefaultUser:
				username = "c##datadog"
				password = "datadog"
			case useSysUser:
				username = "sys"
				password = "datad0g"
			}
		}

		port, err := strconv.Atoi(os.Getenv(portEnvVariable))
		if port == 0 || err != nil {
			port = 1521
		}

		if t != nil {
			require.NotEqualf(t, "", username, "Please set the %s environment variable", userEnvVariable)
			require.NotEqualf(t, "", password, "Please set the %s environment variable", passwordEnvVariable)
			require.NotEqualf(t, "", server, "Please set the %s environment variable", serverEnvVariable)
			require.NotEqualf(t, "", serviceName, "Please set the %s environment variable", serviceNameEnvVariable)
			require.NotEqualf(t, 0, port, "Please set the %s environment variable", portEnvVariable)
		}

		return config.ConnectionConfig{
			Username:    username,
			Password:    password,
			Server:      server,
			Port:        port,
			ServiceName: serviceName,
		}

	}

	switch userType {
	case useLegacyUser:
		return handleRealConnection(useLegacyUser)
	case useSysUser:
		return handleRealConnection(useSysUser)
	case useDoesNotExistUser:
		return config.ConnectionConfig{
			Username:    doesNotExist,
			Password:    doesNotExist,
			Server:      "localhost",
			Port:        60000,
			ServiceName: doesNotExist,
		}
	default:
		return handleRealConnection(useDefaultUser)
	}
}

func newTestCheck(t *testing.T, connectConfig config.ConnectionConfig, instanceConfigAddition string, initConfig string) (Check, *mocksender.MockSender) {
	var err error
	c := Check{}

	connectYaml, err := yaml.Marshal(connectConfig)
	if t != nil {
		require.NoError(t, err)
	}
	instanceConfig := string(connectYaml)
	if instanceConfigAddition != "" {
		instanceConfig = fmt.Sprintf("%s\n%s", instanceConfig, instanceConfigAddition)
	}
	rawInstanceConfig := []byte(instanceConfig)
	rawInitConfig := []byte(initConfig)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err = c.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, rawInitConfig, "oracle_test")
	if t != nil {
		require.NoError(t, err)
	}

	sender := mocksender.NewMockSenderWithSenderManager(c.ID(), senderManager)
	sender.SetupAcceptAll()
	if t != nil {
		assert.Equal(t, c.config.InstanceConfig.Server, connectConfig.Server)
		assert.Equal(t, c.config.InstanceConfig.Port, connectConfig.Port)
		assert.Equal(t, c.config.InstanceConfig.Username, connectConfig.Username)
		assert.Equal(t, c.config.InstanceConfig.Password, connectConfig.Password)
		assert.Equal(t, c.config.InstanceConfig.ServiceName, connectConfig.ServiceName)
		assert.Contains(t, c.configTags, dbmsTag, "c.configTags doesn't contain static tags")
	}

	if oracleLibDir := os.Getenv("ORACLE_TEST_ORACLE_CLIENT_LIB_DIR"); oracleLibDir != "" {
		c.config.InstanceConfig.OracleClientLibDir = oracleLibDir
		c.config.InstanceConfig.OracleClient = true
	}

	return c, sender
}

func newLegacyCheck(t *testing.T, instanceConfigAddition string, initConfig string) (Check, *mocksender.MockSender) {
	// The database user `datadog_legacy` is set up according to
	// https://docs.datadoghq.com/integrations/guide/deprecated-oracle-integration/?tab=linux
	return newTestCheck(t, getConnectData(t, useLegacyUser), instanceConfigAddition, initConfig)
}

func newDefaultCheck(t *testing.T, instanceConfigAddition string, initConfig string) (Check, *mocksender.MockSender) {
	return newTestCheck(t, getConnectData(t, useDefaultUser), instanceConfigAddition, initConfig)
}

func newSysCheck(t *testing.T, instanceConfigAddition string, initConfig string) (Check, *mocksender.MockSender) {
	return newTestCheck(t, getConnectData(t, useSysUser), instanceConfigAddition, initConfig)
}

func newDbDoesNotExistCheck(t *testing.T, instanceConfigAddition string, initConfig string) (Check, *mocksender.MockSender) {
	return newTestCheck(t, getConnectData(t, useDoesNotExistUser), instanceConfigAddition, initConfig)
}

func assertConnectionCount(t *testing.T, c *Check, max int) {
	var n int
	query := "select count(*) from v$session where username = :username"
	err := getWrapper(c, &n, query, strings.ToUpper(c.config.InstanceConfig.Username))
	require.NoError(t, err, "failed to execute the session count query")
	require.LessOrEqual(t, n, max, "too many sessions:")
}
