// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	oracle_common "github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initCheck(t *testing.T, senderManager sender.SenderManager, server string, port int, user string, password string, serviceName string) (Check, error) {
	c := Check{}
	rawInstanceConfig := []byte(fmt.Sprintf(`server: %s
port: %d
username: %s
password: %s
service_name: %s
`, server, port, user, password, serviceName))
	err := c.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "oracle_test")
	require.NoError(t, err)

	return c, err
}

var HOST = "localhost"
var PORT = 1521
var USER = "c##datadog"
var PASSWORD = "datadog"
var SERVICE_NAME = "XE"
var TNS_ALIAS = "XE"
var TNS_ADMIN = "/Users/nenad.noveljic/go/src/github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/testutil/etc/netadmin"

var dbmsTag = "dbms:oracle"

func newRealCheck(t *testing.T, options string) (Check, *mocksender.MockSender) {
	c := Check{}
	config := fmt.Sprintf(`
server: %s
port: %d
username: %s
password: %s
service_name: %s
`, HOST, PORT, USER, PASSWORD, SERVICE_NAME)
	if options != "" {
		config = fmt.Sprintf(`%s
%s`, config, options)
	}
	rawInstanceConfig := []byte(config)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := c.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "oracle_test")
	require.NoError(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(c.ID(), senderManager)
	sender.SetupAcceptAll()
	assert.Equal(t, c.config.InstanceConfig.Server, HOST)
	assert.Equal(t, c.config.InstanceConfig.Port, PORT)
	assert.Equal(t, c.config.InstanceConfig.Username, USER)
	assert.Equal(t, c.config.InstanceConfig.Password, PASSWORD)
	assert.Equal(t, c.config.InstanceConfig.ServiceName, SERVICE_NAME)

	assert.Contains(t, c.configTags, dbmsTag, "c.configTags doesn't contain static tags")

	return c, sender
}

func newLegacyCheck(t *testing.T, options string) (Check, *mocksender.MockSender) {
	// The database user `datadog_legacy` is set up according to
	// https://docs.datadoghq.com/integrations/guide/deprecated-oracle-integration/?tab=linux
	return newTestCheck(t, "datadog_legacy", options)
}

func newTestCheck(t *testing.T, username string, options string) (Check, *mocksender.MockSender) {
	c := Check{}
	config := fmt.Sprintf(`
server: %s
port: %d
username: %s
password: %s
service_name: %s
`, HOST, PORT, username, PASSWORD, SERVICE_NAME)
	if options != "" {
		config = fmt.Sprintf(`%s
%s`, config, options)
	}
	rawInstanceConfig := []byte(config)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := c.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "oracle_test")
	require.NoError(t, err)

	sender := mocksender.NewMockSenderWithSenderManager(c.ID(), senderManager)
	sender.SetupAcceptAll()
	assert.Equal(t, c.config.InstanceConfig.Server, HOST)
	assert.Equal(t, c.config.InstanceConfig.Port, PORT)
	assert.Equal(t, c.config.InstanceConfig.Username, username)
	assert.Equal(t, c.config.InstanceConfig.Password, PASSWORD)
	assert.Equal(t, c.config.InstanceConfig.ServiceName, SERVICE_NAME)

	assert.Contains(t, c.configTags, dbmsTag, "c.configTags doesn't contain static tags")

	return c, sender
}

func skipGodror() bool {
	return os.Getenv("SKIP_GODROR_TESTS") == "1"
}

func getDrivers() []string {
	drivers := []string{oracle_common.GoOra}
	if !skipGodror() {
		drivers = append(drivers, oracle_common.Godror)
	}
	return drivers
}
